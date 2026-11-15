// trash/ を別の Go モジュールにして、`go build ./...` や `go vet ./...`、
// golangci-lint が片付け済みの Go ファイルを読まないようにする。
module trash

go 1.27.3
