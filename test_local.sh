# TODO 本地测试时, 需在本地起个redis供测试使用
go test -cover -coverprofile=/tmp/cover.out -race -count=1 -v ./...
go tool cover -html=/tmp/cover.out -o /tmp/coverage.html
open /tmp/coverage.html