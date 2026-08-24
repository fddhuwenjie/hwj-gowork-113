FROM golang@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd

WORKDIR /workspace

ENV GOTOOLCHAIN=local
ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/tmp/go-build

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN mkdir -p /tmp/source-check \
    && cp -a /workspace/. /tmp/source-check/ \
    && cd /tmp/source-check \
    && go build ./... \
    && rm -rf /tmp/source-check

CMD ["bash"]
