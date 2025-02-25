#!/bin/sh

# docker run --rm -v "$PWD":/usr/src/myapp -w /usr/src/myapp -e CGO_ENABLED=1 -e GOARCH=amd64 golang:1.18 go build -v -o build/harmonylite-linux-amd64 harmonylite.go

CC=x86_64-linux-musl-gcc \
CXX=x86_64-linux-musl-g++ \
GOARCH=amd64 GOOS=linux CGO_ENABLED=1 \
go build -ldflags "-linkmode external -extldflags -static" -o dist/linux/amd64/harmonylite

