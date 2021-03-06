# __release_tag__ golang 1.16 was released 2021-02-16
# __release_tag__ alpine 3.13 was released 2021-01-14

# stage 1: build
FROM golang:1.16 as build
LABEL stage=intermediate
WORKDIR /app
COPY . .
RUN make build

# stage 2: scratch
FROM alpine:3.13 as scratch
COPY --from=build /app/bin/pgcenter /bin/pgcenter
CMD ["pgcenter"]