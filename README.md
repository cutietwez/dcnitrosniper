
# linux build

```sh
go build -ldflags "-s -w" -o isniper *.go && strip -s isniper
```
