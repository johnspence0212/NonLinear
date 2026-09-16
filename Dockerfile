FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /nonlinear ./cmd/nonlinear

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /nonlinear /usr/local/bin/nonlinear
VOLUME /data
ENV DATA_DIR=/data PORT=3333
EXPOSE 3333
ENTRYPOINT ["nonlinear"]
