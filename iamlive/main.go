package main

import (
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
	"github.com/iann0036/iamlive-lambda-extension/iamlive/logsapi"
	"github.com/iann0036/iamlive/iamlivecore"
)

var (
	extensionName    = filepath.Base(os.Args[0]) // extension name has to match the filename
	extensionClient  = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	printPrefix      = fmt.Sprintf("[%s]", extensionName)
	initialQueueSize = int64(5)
)

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
		iamlivecore.RunWithArgs(false, "default", false, "", 0, false, "127.0.0.1", "proxy", "127.0.0.1:10080", "/tmp/iamlive-ca.pem", "/tmp/iamlive-ca.key", "", false, false)
		println(printPrefix, "Proxy stopped")
	}()
	time.Sleep(3 * time.Second)
	println(printPrefix, "Started proxy")

	res, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}
	println(printPrefix, "Register response:", prettyPrint(res))

	go func() {
		logQueue := queue.New(initialQueueSize)

		_, err := agent.NewHttpAgent(logQueue)
		if err != nil {
			log.Fatal(err)
		}

		logsApiClient, err := logsapi.NewClient(fmt.Sprintf("http://%s", os.Getenv("AWS_LAMBDA_RUNTIME_API")))
		if err != nil {
			log.Fatal(err)
		}
		eventTypes := []logsapi.EventType{logsapi.Platform}
		bufferingCfg := logsapi.BufferingCfg{
			MaxItems:  10000,
			MaxBytes:  262144,
			TimeoutMS: 100,
		}
		destination := logsapi.Destination{
			Protocol:   logsapi.HttpProto,
			URI:        logsapi.URI("http://sandbox:1234"),
			HttpMethod: logsapi.HttpPost,
			Encoding:   logsapi.JSON,
		}

		logsApiClient.Subscribe(eventTypes, bufferingCfg, destination, extensionClient.ExtensionID)

		println(printPrefix, "Logs API handler registered")

		for {
			logItem, err := logQueue.Get(1)

			if err != nil {
				log.Fatal(err)
			}

			println(printPrefix, "Received Log API event")
			fmt.Println(logItem)
		}
	}()

	// Will block until shutdown event is received or cancelled via the context.
	processEvents(ctx)
}

func processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			println(printPrefix, "Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				println(printPrefix, "Error:", err)
				println(printPrefix, "Exiting")
				return
			}
			println(printPrefix, "Received event:", prettyPrint(res))

			// Exit if we receive a SHUTDOWN event
			if res.EventType == extension.Shutdown {
				println(printPrefix, "Received SHUTDOWN event")
				println(printPrefix, "Exiting")
				return
			}

			time.Sleep(5 * time.Second)
			println(printPrefix, "Result IAM Policy:")
			println(string(iamlivecore.GetPolicyDocument()))

		}
	}
}

func prettyPrint(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return ""
	}
	return string(data)
}
