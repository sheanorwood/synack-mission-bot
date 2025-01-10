
# Description

  This program periodically polls the Synack platform for two things:

  1. Available missions (tasks):
       - If a mission can be claimed, the script claims it.
       - Upon a successful claim, a notification is printed to stdout.
    
  2. 2. Unregistered targets:
       - Any newly discovered unregistered targets are automatically signed up for.
  
  Target: An overall listing or program you can sign up for (i.e., an organization’s scope). 
  “targets” are fetched from the /api/targets endpoint and represent entire programs or listings 
  you can register to test. 
	
  Mission (Task): A specific, claimable assignment under a particular target. In the code, these are retrieved 
  from /api/tasks/v2/tasks and can be “claimed” once you have access to that target. Each mission (or “task”) 
  usually has a unique ID, status (e.g., PUBLISHED), and time window in which it can be claimed for completion.
  
## Steps

1. Build the binary with a custom output name:
```go build -o mission_bot mission_bot.go```

2. Move it into a folder on your PATH. A common choice is /usr/local/bin.
```sudo mv mission_bot /usr/local/bin/```

3. Verify permissions (optional). By default, it should be executable, but you can ensure it with:
```sudo chmod +x /usr/local/bin/mission_bot```

4. Run the program
```mission_bot -t "YOUR_SESSION_TOKEN_HERE"```

```
Usage of mission_bot:
  -t <token>     Provide your session token (JWT) for authentication with the Synack platform.
                 This token is used for polling tasks/targets and claiming missions.

 If the session token expires (HTTP 401), the script prompts you to enter a new token
  interactively and then continues operating with the refreshed token.

Example:
  ./mission_bot -t "YOUR_SESSION_TOKEN_HERE" | notify -silent
   mission_bot -t "YOUR_SESSION_TOKEN_HERE" | notify -silent # if in path
