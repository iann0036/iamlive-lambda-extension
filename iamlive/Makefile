build:
	GOOS=linux GOARCH=amd64 go build -o bin/extensions/iamlive

build-IamliveExtensionLayer:
	GOOS=linux GOARCH=amd64 go build -o $(ARTIFACTS_DIR)/extensions/iamlive
	chmod +x $(ARTIFACTS_DIR)/extensions/iamlive

run-IamliveExtensionLayer:
	go run iamlive/main.go
