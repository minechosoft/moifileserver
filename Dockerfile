From golang:latest

ADD . /go/src/github.com/minechosoft/moifileserver

# Build the contact_registry command inside the container.

RUN go install github.com/minechosoft/moifileserver

# Run the contact_registry command when the container starts.

ENTRYPOINT /go/bin/moifileserver

# http server listens on port 8080.

EXPOSE 8088
