@echo off
wsl -d Ubuntu bash -c "cd /mnt/c/Users/macie/github/kubeEphemerals && go test -v ./internal/ui/... -run Templates 2>&1" > C:\Users\macie\github\kubeEphemerals\test_output.txt
type C:\Users\macie\github\kubeEphemerals\test_output.txt
