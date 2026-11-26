# stage 1: build
FROM golang:1.26.10-alpine AS build
LABEL stage=intermediate
WORKDIR /app
COPY . .
RUN apk --no-cache add git make
RUN make build

# stage 2: final image
FROM alpine:3.21 AS final
RUN apk --no-cache add ca-certificates
COPY --from=build /app/bin/pgcenter /bin/pgcenter
CMD ["pgcenter"]
