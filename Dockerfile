FROM golang:1.12.0 AS builder
RUN mkdir /artifact
WORKDIR /artifact
ADD . .
ENV CGO_ENABLED=0
RUN go build -o main

FROM centurylink/ca-certs AS runtime
COPY --from=builder /artifact/config ./config
COPY --from=builder /artifact/data ./data
COPY --from=builder /artifact/main ./main

EXPOSE 1337

CMD ["./main"]
