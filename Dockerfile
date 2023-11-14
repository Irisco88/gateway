# syntax=docker/dockerfile:1.4
ARG GO_VERSION="1.20"
ARG GOPROXYURL="https://goproxy.io"
ARG COMPRESS="true"
ARG COMPANY_HOST="github.com/irisco88"
ARG GITHUB_TOKEN

FROM golang:${GO_VERSION}-alpine AS builder
# install packages
RUN sed -i 's#dl-cdn.alpinelinux.org#alpine.global.ssl.fastly.net#g' /etc/apk/repositories
RUN apk --no-cache add --update ca-certificates tzdata upx git

# config git
ARG COMPANY_HOST
ARG GITHUB_TOKEN
ENV GOPRIVATE="${COMPANY_HOST}/*"
RUN git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@${COMPANY_HOST}".insteadOf "https://${COMPANY_HOST}"

# copy source code
WORKDIR /build
COPY . .

# Get all of our dependencies
ARG GOPROXYURL
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod GOPROXY="${GOPROXYURL}" go mod download -x
# compile project
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-s -w" -a -installsuffix cgo -o ./bin/gateway ./cmd/...

ARG COMPRESS
RUN mkdir -p /final && \
    if [ "$COMPRESS" = "true" ] ;then upx --best --lzma -o /final/gateway ./bin/gateway ;else cp ./bin/release/gateway /final; fi

FROM scratch AS final

WORKDIR /production
COPY --from=builder /final .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

ENTRYPOINT ["./gateway"]
