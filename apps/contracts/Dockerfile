FROM golang:1.25.4-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler=3.21.12-3+deb12u1 libprotobuf-dev=3.21.12-3+deb12u1
ENV GOBIN=/usr/local/bin
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
ENV PATH="/usr/local/bin:${PATH}"
