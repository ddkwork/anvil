#!/bin/bash
set -x

pushd src/anvil
go test ./...
