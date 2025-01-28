package main

import (
    "bufio"
    "bytes"
    "crypto/tls"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"
)

// Task represents the JSON structure for tasks returned by Synack.
type Task struct {
    ID              string `json:"id"`
    CampaignUid     string `json:"campaignUid"`
    ListingUid      string `json:"listingUid"`
    OrganizationUid string `json:"organizationUid"`
    // Optionally, if the API returns a payout or similar, you could add:
    // Payout          float64 `json:"payout"`
}

// Target represents the JSON structure for unregistered targets.
type Target struct {
    Slug string `json:"slug"`
}

// tasksEndpoint is the default endpoint for retrieving tasks.
var tasksEndpoint = "https://platform.synack.com/api/tasks/v2/tasks"

// globalHTTPClient returns an HTTP client with InsecureSkipVerify (for demo).
// In production, handle certificates properly.
func globalHTTPClient() *http.Client {
    tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
    return &http.Client{Transport: tr}
}

// init overrides the default flag usage to display a custom help message.
func init() {
    flag.Usage = func() {
        fmt.Fprintf(os.Stderr, `
Usage of %s:
  -t <token>    Provide your session token (JWT) for authentication with the Synack platform.
  -v            Enable verbose logging.

Description:
  This program periodically polls the Synack platform for two things:

    1. Available missions (tasks):
       - If a mission can be claimed, the script claims it.
       - If 403 is received 5 times in a row while claiming tasks, it gracefully stops.
       - If 401 (unauthorized) is encountered, it prompts for a new token.
       - Waits 5 seconds between each claimed task, and 30 seconds between polling cycles.

    2. Unregistered targets:
       - Checks every 5 minutes. Any newly discovered unregistered targets are automatically
         signed up for.

Example:
  synack-mission-bot -t "YOUR_SESSION_TOKEN_HERE" -v

Flags:
`, os.Args[0])
        flag.PrintDefaults()
    }
}

// getTasks retrieves tasks from Synack.
func getTasks(token string) ([]Task, error) {
    client := globalHTTPClient()

    req, err := http.NewRequest("GET", tasksEndpoint, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    // Query params
    q := req.URL.Query()
    q.Add("perPage", "20")
    q.Add("viewed", "true")
    q.Add("page", "1")
    q.Add("status", "PUBLISHED")
    q.Add("sort", "CLAIMABLE")
    q.Add("sortDir", "DESC")
    q.Add("includeAssignedBySynackUser", "false")
    req.URL.RawQuery = q.Encode()

    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    switch resp.StatusCode {
    case http.StatusOK:
        var tasks []Task
        if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
            return nil, err
        }
        return tasks, nil

    case http.StatusUnauthorized:
        return nil, fmt.Errorf("unauthorized (401)")

    case 429:
        // If we hit a 429, implement a simple backoff or check Retry-After
        retryAfter := resp.Header.Get("Retry-After")
        if retryAfter == "" {
            fmt.Println("Got 429 Too Many Requests. Sleeping 30 seconds.")
            time.Sleep(30 * time.Second)
        } else {
            if secs, err := strconv.Atoi(retryAfter); err == nil {
                fmt.Printf("Got 429 Too Many Requests, waiting %d seconds...\n", secs)
                time.Sleep(time.Duration(secs) * time.Second)
            } else {
                time.Sleep(30 * time.Second)
            }
        }
        // Retry once after waiting
        return getTasks(token)

    default:
        return nil, fmt.Errorf("failed to retrieve tasks, status code: %d", resp.StatusCode)
    }
}

// postClaimTask attempts to claim a specific task.
func postClaimTask(token string, task Task) error {
    client := globalHTTPClient()
    url := fmt.Sprintf(
        "https://platform.synack.com/api/tasks/v1/organizations/%s/listings/%s/campaigns/%s/tasks/%s/transitions",
        task.OrganizationUid, task.ListingUid, task.CampaignUid, task.ID,
    )

    payload := []byte(`{"type": "CLAIM"}`)
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    switch resp.StatusCode {
    case http.StatusCreated:
        fmt.Println("Mission claimed successfully.")
        return nil
    case http.StatusPreconditionFailed:
        return fmt.Errorf("Mission cannot be claimed anymore (412)")
    case http.StatusUnauthorized:
        return fmt.Errorf("Unauthorized (401)")
    case http.StatusForbidden:
        return fmt.Errorf("Failed to claim task, status code: 403")
    default:
        return fmt.Errorf("Failed to claim task, status code: %d", resp.StatusCode)
    }
}

// pollUnregisteredTargets checks unregistered targets every 5 minutes and signs up for new ones.
func pollUnregisteredTargets(token string, knownSlugs *sync.Map, tokenChan chan string, verbose bool) {
    for {
        select {
        case newToken := <-tokenChan:
            token = newToken
        default:
            // continue using current token
        }

        // Verbose logging
        if verbose {
            log.Println("Checking for unregistered targets...")
        }

        targets, err := getUnregisteredTargets(token)
        if err != nil {
            if strings.Contains(err.Error(), "401") {
                newToken := refreshToken()
                tokenChan <- newToken
                token = newToken
                continue
            }
            log.Println(err)
        } else {
            for _, t := range targets {
                if _, loaded := knownSlugs.LoadOrStore(t.Slug, true); !loaded {
                    // Found a new slug, sign up
                    err := signupTarget(token, t.Slug)
                    if err != nil {
                        log.Println(err)
                    }
                }
            }
        }

        // Sleep 5 minutes (300 seconds) before checking again
        time.Sleep(5 * time.Minute)
    }
}

// getUnregisteredTargets retrieves the unregistered targets from Synack.
func getUnregisteredTargets(token string) ([]Target, error) {
    client := globalHTTPClient()
    url := "https://platform.synack.com/api/targets?filter%5Bprimary%5D=unregistered&filter%5Bsecondary%5D=all&filter%5Bcategory%5D=all&filter%5Bindustry%5D=all&filter%5Bpayout_status%5D=all&sorting%5Bfield%5D=onboardedAt&sorting%5Bdirection%5D=desc&pagination%5Bpage%5D=1&pagination%5Bper_page%5D=15"

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    switch resp.StatusCode {
    case http.StatusOK:
        var targets []Target
        if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
            return nil, err
        }
        return targets, nil
    case http.StatusUnauthorized:
        return nil, fmt.Errorf("unauthorized (401)")
    case 429:
        fmt.Println("Got 429 Too Many Requests on targets. Sleeping 30 seconds.")
        time.Sleep(30 * time.Second)
        // Retry once after waiting
        return getUnregisteredTargets(token)
    default:
        return nil, fmt.Errorf("failed to retrieve unregistered targets, status code: %d", resp.StatusCode)
    }
}

// signupTarget attempts to sign up for a target using its slug.
func signupTarget(token, slug string) error {
    client := globalHTTPClient()
    url := fmt.Sprintf("https://platform.synack.com/api/targets/%s/signup", slug)
    payload := []byte(`{"ResearcherListing": {"terms": 1}}`)

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusOK {
        fmt.Printf("Signed up for target %s successfully.\n", slug)
        return nil
    } else if resp.StatusCode == http.StatusUnauthorized {
        return fmt.Errorf("unauthorized (401)")
    } else if resp.StatusCode == 429 {
        fmt.Println("Got 429 Too Many Requests on signup. Sleeping 30 seconds.")
        time.Sleep(30 * time.Second)
        // Retry once
        return signupTarget(token, slug)
    }

    return fmt.Errorf("failed to sign up for target %s, status code: %d", slug, resp.StatusCode)
}

// refreshToken prompts the user to enter a new token.
func refreshToken() string {
    fmt.Print("Token expired or invalid. Please enter a new token:\n> ")
    reader := bufio.NewReader(os.Stdin)
    newToken, _ := reader.ReadString('\n')
    return strings.TrimSpace(newToken)
}

// mainLoop continuously polls tasks, attempts to claim them, and gracefully stops
// if 403 is encountered 5 times in a row. If verbose is set, it logs each check.
func mainLoop(token string, tokenChan chan string, verbose bool) {
    var consecutive403Count int

    for {
        select {
        case newToken := <-tokenChan:
            token = newToken
        default:
            // no token update
        }

        if verbose {
            log.Println("Checking for available missions...")
        }

        tasks, err := getTasks(token)
        if err != nil {
            if strings.Contains(err.Error(), "401") {
                newToken := refreshToken()
                tokenChan <- newToken
                token = newToken
                consecutive403Count = 0
                continue
            }
            log.Println(err)
        } else {
            // Process tasks
            for _, task := range tasks {
                err := postClaimTask(token, task)
                if err != nil {
                    // If it's a 403, increment counter
                    if strings.Contains(err.Error(), "403") {
                        consecutive403Count++
                        log.Printf("Got 403. Current consecutive403Count = %d\n", consecutive403Count)
                        if consecutive403Count >= 5 {
                            log.Println("Received 403 five times in a row. Stopping the bot.")
                            return // Graceful exit
                        }
                    } else if strings.Contains(err.Error(), "401") {
                        newToken := refreshToken()
                        tokenChan <- newToken
                        token = newToken
                        consecutive403Count = 0
                        break
                    } else {
                        log.Println(err)
                    }
                } else {
                    // Success, reset the 403 counter
                    consecutive403Count = 0
                    fmt.Printf("Claimed task %s successfully.\n", task.ID)
                    // Sleep 5s per your existing logic
                    time.Sleep(5 * time.Second)
                }
            }
        }

        // Sleep 30s between task polls
        time.Sleep(15 * time.Second)
    }
}

func main() {
    tokenFlag := flag.String("t", "", "Session token for authentication")
    verboseFlag := flag.Bool("v", false, "Enable verbose logging")
    flag.Parse()

    if *tokenFlag == "" {
        flag.Usage()
        os.Exit(1)
    }

    token := *tokenFlag
    verbose := *verboseFlag

    // Known slugs map to track which slugs have been processed
    knownSlugs := &sync.Map{}

    // Channel to communicate token updates between goroutines
    tokenChan := make(chan string)

    // Start polling unregistered targets every 5 mins
    go pollUnregisteredTargets(token, knownSlugs, tokenChan, verbose)

    // Start the main loop to poll tasks and claim them
    mainLoop(token, tokenChan, verbose)
}