package main

import (
	"fmt"
	"os"

	"shu/internal/cli"
	"shu/internal/daemon"
	"shu/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "server":
		must(server.Run())
	case "migrate":
		must(server.RunMigrate())
	case "daemon":
		must(daemon.Run(os.Args[2:]))
	case "init-user":
		must(server.InitUser(os.Args[2:]))
	case "workspace", "token", "executor", "resource", "work", "agent", "issue", "comment", "attachment", "events", "inbox", "squad", "autopilot":
		must(cli.Run(os.Args[1:]))
	default:
		usage()
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`shu: API-first agent work platform

Server:
  shu server
  shu migrate
  shu init-user <name>                # create token directly in DB

Daemon:
  shu daemon start

CLI:
  shu workspace create <slug> <name>
  shu workspace list
  shu token list|create [name]|revoke <id>
  shu executor list
  shu resource create <kind> <locator>
  shu resource list
  shu work create <title> [--prompt text] [--resource id] [--provider codex]
  shu work list|get <id>|artifacts <id>|cancel <id>|watch <id>
  shu agent create <name> <provider>
  shu agent list
  shu issue create <title> [--description text] [--assignee agent] [--priority p]
  shu issue list|get <id>|update <id> [--status s]
  shu issue comment <id> <body>|comments <id>|timeline <id>
  shu attachment upload <path> --issue <id>
  shu inbox list|read <id>|archive <id>
  shu squad create <name> <leader-agent>
  shu squad add <squad> <agent>
  shu autopilot create <name> <assignee> <interval-seconds> <prompt>
  shu autopilot list|run <id>
  shu events watch`)
}
