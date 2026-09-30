# Single-binary image: build the server, ship it with PostgreSQL client tools
# so backup.sh/restore.sh work inside the container too.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
RUN CGO_ENABLED=0 go build -o /aihub ./cmd/aihub

FROM alpine:3.20
RUN apk add --no-cache ca-certificates postgresql-client tzdata
COPY --from=build /aihub /usr/local/bin/aihub
ENV AIHUB_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/aihub"]
