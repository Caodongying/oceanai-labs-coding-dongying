FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM build AS test
RUN go test ./... && go vet ./...

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /server /server
ENV DB_PATH=/data/certificates.db LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/server"]
