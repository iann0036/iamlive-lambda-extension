build:
	GOOS=linux GOARCH=amd64 go build -o bin/extensions/iamlive main.go

build-IamliveExtensionLayer:
	GOOS=linux GOARCH=amd64 go build -o $(ARTIFACTS_DIR)/extensions/iamlive main.go
	chmod +x $(ARTIFACTS_DIR)/extensions/iamlive

run-IamliveExtensionLayer:
	go run iamlive/main.go
