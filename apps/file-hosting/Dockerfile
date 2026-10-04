FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/file-hosting ./cmd/file-hosting

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/file-hosting /usr/local/bin/file-hosting
USER app
EXPOSE 1447
ENV ADDR=:1447
ENV FILES_PATH=/data
VOLUME ["/data"]
ENTRYPOINT ["file-hosting"]
