FROM golang:1.20.0-alpine3.17 AS build
WORKDIR /src
COPY . /src
RUN go build .

FROM alpine:3.17.1 AS production
COPY --from=build /src/equinix-billing-tools /usr/local/bin
ENTRYPOINT ["/usr/local/bin/equinix-billing-tools"]
CMD -h
