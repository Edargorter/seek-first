FROM golang:1.19
WORKDIR /app
COPY go.mod go.sum ./
RUN mkdir -p ./data
COPY data/* ./data/
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /seek-first
CMD ["/seek-first"]
