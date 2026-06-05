package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"shu/internal/apiclient"
)

func Run(args []string) error {
	c := args[0]
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	switch c + " " + sub {
	case "workspace create":
		need(args, 4)
		return printReq("POST", "/api/workspaces", map[string]string{"slug": args[2], "name": args[3]})
	case "workspace list":
		return printReq("GET", "/api/workspaces", nil)
	case "token list":
		return printReq("GET", "/api/tokens", nil)
	case "token create":
		name := "cli"
		if len(args) > 2 {
			name = args[2]
		}
		return printReq("POST", "/api/tokens", map[string]string{"name": name})
	case "token revoke":
		need(args, 3)
		return printReq("DELETE", "/api/tokens/"+args[2], nil)
	case "executor list":
		return printReq("GET", "/api/executors", nil)
	case "resource create":
		need(args, 4)
		return printReq("POST", "/api/resources", map[string]string{"kind": args[2], "locator": args[3]})
	case "resource list":
		return printReq("GET", "/api/resources", nil)
	case "work create":
		need(args, 3)
		body := map[string]any{"title": args[2], "provider": "codex"}
		for i := 3; i < len(args); i++ {
			if args[i] == "--prompt" && i+1 < len(args) {
				body["prompt"] = args[i+1]
				i++
			}
			if args[i] == "--resource" && i+1 < len(args) {
				body["resourceID"] = args[i+1]
				body["resource_id"] = args[i+1]
				i++
			}
			if args[i] == "--provider" && i+1 < len(args) {
				body["provider"] = args[i+1]
				i++
			}
		}
		return printReq("POST", "/api/work", body)
	case "work list":
		return printReq("GET", "/api/work", nil)
	case "work get":
		need(args, 3)
		return printReq("GET", "/api/work/"+args[2], nil)
	case "work artifacts":
		need(args, 3)
		return printReq("GET", "/api/work/"+args[2]+"/artifacts", nil)
	case "work watch":
		need(args, 3)
		return watchWork(args[2])
	case "issue create":
		need(args, 3)
		body := map[string]string{"title": args[2]}
		for i := 3; i < len(args); i++ {
			if args[i] == "--description" && i+1 < len(args) {
				body["description"] = args[i+1]
				i++
			}

			if args[i] == "--assignee" && i+1 < len(args) {
				body["assignee"] = args[i+1]
				i++
			}
			if args[i] == "--priority" && i+1 < len(args) {
				body["priority"] = args[i+1]
				i++
			}
		}
		return printReq("POST", "/api/issues", body)
	case "issue list":
		return printReq("GET", "/api/issues", nil)
	case "issue get":
		need(args, 3)
		return printReq("GET", "/api/issues/"+args[2], nil)
	case "issue update":
		need(args, 3)
		body := map[string]string{}
		for i := 3; i < len(args); i++ {
			if args[i] == "--title" && i+1 < len(args) {
				body["title"] = args[i+1]
				i++
			}
			if args[i] == "--description" && i+1 < len(args) {
				body["description"] = args[i+1]
				i++
			}
			if args[i] == "--status" && i+1 < len(args) {
				body["status"] = args[i+1]
				i++
			}
			if args[i] == "--priority" && i+1 < len(args) {
				body["priority"] = args[i+1]
				i++
			}
		}
		return printReq("PATCH", "/api/issues/"+args[2], body)
	case "issue comments":
		need(args, 3)
		return printReq("GET", "/api/issues/"+args[2]+"/comments", nil)
	case "issue comment":
		need(args, 4)
		return printReq("POST", "/api/issues/"+args[2]+"/comments", map[string]string{"body": args[3]})
	case "issue timeline":
		need(args, 3)
		return printReq("GET", "/api/issues/"+args[2]+"/timeline", nil)
	case "attachment upload":
		need(args, 4)
		return uploadAttachment(args)
	case "attachment get":
		need(args, 3)
		return printReq("GET", "/api/attachments/"+args[2], nil)
	case "events watch":
		return watchEvents()
	case "inbox list":
		return printReq("GET", "/api/inbox", nil)
	case "inbox read":
		need(args, 3)
		return printReq("PATCH", "/api/inbox/"+args[2]+"/read", nil)
	case "inbox archive":
		need(args, 3)
		return printReq("PATCH", "/api/inbox/"+args[2]+"/archive", nil)
	case "squad create":
		need(args, 4)
		return printReq("POST", "/api/squads", map[string]string{"name": args[2], "leader": args[3]})
	case "squad add":
		need(args, 4)
		return printReq("POST", "/api/squads/"+args[2]+"/members", map[string]string{"agent": args[3]})
	case "autopilot create":
		need(args, 6)
		sec, _ := strconv.Atoi(args[4])
		return printReq("POST", "/api/autopilots", map[string]any{"name": args[2], "assignee": args[3], "intervalSeconds": sec, "prompt": args[5]})
	case "autopilot list":
		return printReq("GET", "/api/autopilots", nil)
	case "autopilot run":
		need(args, 3)
		return printReq("POST", "/api/autopilots/"+args[2]+"/run", nil)
	default:
		usage()
		return nil
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: shu <command> [args]")
}

func need(args []string, n int) {
	if len(args) < n {
		usage()
		os.Exit(2)
	}
}

func uploadAttachment(args []string) error {
	issueID := ""
	commentID := ""
	filePath := args[2]
	for i := 3; i < len(args); i++ {
		if args[i] == "--issue" && i+1 < len(args) {
			issueID = args[i+1]
			i++
		}
		if args[i] == "--comment" && i+1 < len(args) {
			commentID = args[i+1]
			i++
		}
	}
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return printReq("POST", "/api/attachments", map[string]string{
		"issueId": issueID, "commentId": commentID, "fileName": filepath.Base(filePath),
		"contentBase64": base64.StdEncoding.EncodeToString(b),
	})
}

func printReq(m, p string, b any) error {
	out, err := apiclient.Request(m, p, b)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, out, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(out))
	}
	return nil
}
func watchEvents() error {
	req, _ := http.NewRequest("GET", apiclient.APIBase()+"/api/events/stream?workspace="+apiclient.Workspace(), nil)
	if apiclient.Token() != "" {
		req.Header.Set("Authorization", "Bearer "+apiclient.Token())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}
func watchWork(id string) error {
	go watchEvents()
	seen := map[string]bool{}
	for {
		b, err := apiclient.Request("GET", "/api/work/"+id+"/artifacts", nil)
		if err == nil {
			var arr []map[string]any
			_ = json.Unmarshal(b, &arr)
			for _, e := range arr {
				artifactID, _ := e["id"].(string)
				if artifactID != "" && !seen[artifactID] {
					seen[artifactID] = true
					fmt.Printf("[%s] %v\n", e["type"], e["data"])
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}
