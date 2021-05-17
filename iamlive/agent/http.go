package agent

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-collections/go-datastructures/queue"
	"github.com/iann0036/iamlive-lambda-extension/iamlive/logsapi"
)

// LogsApiHttpListener is used to listen to the Logs API using HTTP
type LogsApiHttpListener struct {
	httpServer *http.Server
	// logQueue is a synchronous queue and is used to put the received logs to be consumed later (see main)
	logQueue *queue.Queue
}

// NewLogsApiHttpListener returns a LogsApiHttpListener with the given log queue
func NewLogsApiHttpListener(lq *queue.Queue) (*LogsApiHttpListener, error) {

	return &LogsApiHttpListener{
		httpServer: nil,
		logQueue:   lq,
	}, nil
}

// Start initiates the server in a goroutine where the logs will be sent
func (s *LogsApiHttpListener) Start() (bool, error) {
	address := "sandbox:1234"
	s.httpServer = &http.Server{Addr: address}
	http.HandleFunc("/", s.http_handler)
	go func() {
		err := s.httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("Unexpected stop on Http Server: %v", err)
			s.Shutdown()
		} else {
			log.Fatalf("Http Server closed %v", err)
		}
	}()
	return true, nil
}

// http_handler handles the requests coming from the Logs API.
// Everytime Logs API sends logs, this function will read the logs from the response body
// and put them into a synchronous queue to be read by the main goroutine.
// Logging or printing besides the error cases below is not recommended if you have subscribed to receive extension logs.
// Otherwise, logging here will cause Logs API to send new logs for the printed lines which will create an infinite loop.
func (h *LogsApiHttpListener) http_handler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatalf("Error reading body: %+v", err)
		return
	}

	// Puts the log message into the queue
	err = h.logQueue.Put(string(body))
	if err != nil {
		log.Fatalf("Can't push logs to destination: %v", err)
	}
}

// Shutdown terminates the HTTP server listening for logs
func (s *LogsApiHttpListener) Shutdown() {
	if s.httpServer != nil {
		ctx, _ := context.WithTimeout(context.Background(), 1*time.Second)
		err := s.httpServer.Shutdown(ctx)
		if err != nil {
			log.Fatalf("Failed to shutdown http server gracefully %s", err)
		} else {
			s.httpServer = nil
		}
	}
}

// HttpAgent has the listener that receives the logs and the logger that handles the received logs
type HttpAgent struct {
	listener *LogsApiHttpListener
}

// NewHttpAgent returns an agent to listen and handle logs coming from Logs API for HTTP
// Make sure the agent is initialized by calling Init(agentId) before subscription for the Logs API.
func NewHttpAgent(jq *queue.Queue) (*HttpAgent, error) {

	logsApiListener, err := NewLogsApiHttpListener(jq)
	if err != nil {
		return nil, err
	}

	return &HttpAgent{
		listener: logsApiListener,
	}, nil
}

// Init initializes the configuration for the Logs API and subscribes to the Logs API for HTTP
func (a HttpAgent) Init(agentID string) error {
	extensions_api_address, ok := os.LookupEnv("AWS_LAMBDA_RUNTIME_API")
	if !ok {
		return errors.New("AWS_LAMBDA_RUNTIME_API is not set")
	}

	logsApiBaseUrl := fmt.Sprintf("http://%s", extensions_api_address)

	logsApiClient, err := logsapi.NewClient(logsApiBaseUrl)
	if err != nil {
		return err
	}

	_, err = a.listener.Start()
	if err != nil {
		return err
	}

	eventTypes := []logsapi.EventType{logsapi.Platform}
	bufferingCfg := logsapi.BufferingCfg{
		MaxItems:  10000,
		MaxBytes:  262144,
		TimeoutMS: 100,
	}
	if err != nil {
		return err
	}
	destination := logsapi.Destination{
		Protocol:   logsapi.HttpProto,
		URI:        logsapi.URI("http://sandbox:1234"),
		HttpMethod: logsapi.HttpPost,
		Encoding:   logsapi.JSON,
	}

	_, err = logsApiClient.Subscribe(eventTypes, bufferingCfg, destination, agentID)
	return err
}

// Shutdown finalizes the logging and terminates the listener
func (a *HttpAgent) Shutdown() {
	a.listener.Shutdown()
}
