.PHONY: build
build: nsmd icmp-responder-nse

.PHONY: nsmd
nsmd:
	CGO_ENABLED=0 GOOS=linux go build -ldflags '-extldflags "-static"' -o $(GOPATH)/bin/nsmd ./controlplane/cmd/nsmd/nsmd.go

.PHONY: icmp-responder-nse
icmp-responder-nse:
	CGO_ENABLED=0 GOOS=linux go build -ldflags '-extldflags "-static"' -o $(GOPATH)/bin/icmp-responder-nse ./examples/cmd/icmp-responder-nse

