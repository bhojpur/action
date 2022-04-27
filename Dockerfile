# File generated by github.com/posener/goaction. DO NOT EDIT.

FROM golang:1.18.1-alpine3.15
RUN apk add git docker wget curl jq

COPY . /home/src
WORKDIR /home/src
RUN go build -o /bin/action ./

ENTRYPOINT [ "/bin/action" ]
