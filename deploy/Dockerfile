FROM node:24-alpine AS web
WORKDIR /work
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build

FROM golang:1.27-alpine AS server
WORKDIR /work
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /work/dist ./webassets/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /aihub ./cmd/aihub

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /aihub /aihub
ENV AIHUB_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/aihub"]
