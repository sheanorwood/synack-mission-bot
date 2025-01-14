
# Description

  This program periodically polls the Synack platform for two things:

  1. Available missions (tasks):
       - If a mission can be claimed, the script claims it.
       - Upon a successful claim, a notification is printed to stdout.
       - The bot will stop after 5 consecutive 403 responses from the server. Usually, once all missions can be claimed for your level.
    
  2. 2. Unregistered targets:
       - Any newly discovered unregistered targets are automatically signed up for.
  
  Target: An overall listing or program you can sign up for (i.e., an organization’s scope). 
  “targets” are fetched from the /api/targets endpoint and represent entire programs or listings 
  you can register to test. 
	
  Mission (Task): A specific, claimable assignment under a particular target. In the code, these are retrieved 
  from /api/tasks/v2/tasks and can be “claimed” once you have access to that target. Each mission (or “task”) 
  usually has a unique ID, status (e.g., PUBLISHED), and time window in which it can be claimed for completion.
  
## Steps

1. Install with Go
```go install github.com/sheanorwood/synack-mission-bot```

2. Run the program {-v for verbose output}
```synack-mission-bot -t "YOUR_SESSION_TOKEN_HERE" -v```


Usage of mission_bot:
  -t <token>     Provide your session token (JWT) for authentication with the Synack platform.
                 This token is used for polling tasks/targets and claiming missions.

  -v            Enable verbose logging to STDOUT

 If the session token expires (HTTP 401), the script prompts you to enter a new token
  interactively and then continues operating with the refreshed token.

Example:

synack-mission-bot -t "YOUR_SESSION_TOKEN_HERE" | notify -silent

