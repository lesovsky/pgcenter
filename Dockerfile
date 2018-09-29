FROM golang:1.11

WORKDIR /go/src/pgcenter

ADD . .

RUN export GO111MODULE="on" && export GOPATH="/go" && make && make install

ENTRYPOINT ["pgcenter"]
