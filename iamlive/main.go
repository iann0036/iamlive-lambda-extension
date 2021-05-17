package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang-collections/go-datastructures/queue"
	"github.com/iann0036/iamlive-lambda-extension/iamlive/agent"
	"github.com/iann0036/iamlive-lambda-extension/iamlive/extension"
	"github.com/iann0036/iamlive/iamlivecore"
)

var (
	extensionName    = filepath.Base(os.Args[0]) // extension name has to match the filename
	extensionClient  = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	printPrefix      = fmt.Sprintf("[%s]", extensionName)
	initialQueueSize = int64(5)
	logsApiAgent     *agent.HttpAgent
)

type ParsedLogItem struct {
	Type string `json:"type"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		cancel()
		println(printPrefix, "Received", s)
		println(printPrefix, "Exiting")
	}()

	os.Setenv("HTTP_PROXY", "")
	os.Setenv("HTTPS_PROXY", "")

	go func() {
		iamlivecore.RunWithArgs(false, "default", false, "", 0, false, "127.0.0.1", "proxy", "127.0.0.1:10080", "/tmp/iamlive-ca.pem", "/tmp/iamlive-ca.key", "", true, false)
		println(printPrefix, "Proxy stopped")
	}()
	time.Sleep(3 * time.Second)
	println(printPrefix, "Started proxy")

	_, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}

	go func() {
		logQueue := queue.New(initialQueueSize)

		logsApiAgent, err = agent.NewHttpAgent(logQueue)
		if err != nil {
			log.Fatal(err)
		}

		agentID := extensionClient.ExtensionID
		err = logsApiAgent.Init(agentID)
		if err != nil {
			log.Fatal(err)
		}

		for {
			logItems, err := logQueue.Get(1)
			if err != nil {
				log.Fatal(err)
			}

			var parsedLogItems *[]ParsedLogItem

			for _, logItem := range logItems {
				err = json.Unmarshal([]byte(logItem.(string)), &parsedLogItems)
				if err != nil {
					log.Fatal(err)
				}

				for _, parsedLogItem := range *parsedLogItems {
					if parsedLogItem.Type == "platform.runtimeDone" {
						compactedBuffer := new(bytes.Buffer)
						json.Compact(compactedBuffer, iamlivecore.GetPolicyDocument())
						println(printPrefix, "Result IAM Policy:", compactedBuffer.String())
					}
				}
			}
		}
	}()

	processEvents(ctx)
}

func processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			println(printPrefix, "Context Done")
			return
		default:
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				println(printPrefix, "Error:", err)
				println(printPrefix, "Exiting")
				return
			}

			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				println(printPrefix, "Received SHUTDOWN event")
				time.Sleep(500 * time.Millisecond)
				logsApiAgent.Shutdown()
				return
			}
		}
	}
}
