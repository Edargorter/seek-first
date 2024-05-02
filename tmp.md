# Gowebgo
A CLI web proxy and request inspector/editor written in Golang, adapting the [go-httpproxy](https://pkg.go.dev/github.com/go-httpproxy/httpproxy) library (source available [here](https://github.com/go-httpproxy/httpproxy)).

## CA certificate
1. Download ca_cert.pem from http://127.0.0.1:8081/cert using
```
curl http://127.0.0.1:8081/cert > ca_cert.pem
```
2. Import ca_cert.pem into browser CA certificates.

## User Interface 

![User Interface](images/ui.png "User Interface")

Yes, the name is a reference to Spiderman 1 (with Toby Maguire).

![Go web go](images/go-web-go.gif "Go web go!")
