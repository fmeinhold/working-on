INSTALL_PATH?=/usr/local/bin

build:
	go build -o bin/wo -ldflags="-s -w" main.go
	go run main.go completion fish >${HOME}/.config/fish/completions/wo.fish

run:
	go run main.go


install: build
	sudo install bin/wo ${INSTALL_PATH}
