#!/bin/bash
cd /mnt/c/Users/macie/github/kubeEphemerals
go test -v ./internal/ui/... -run Templates 2>&1
