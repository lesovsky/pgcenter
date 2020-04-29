PROGRAM_NAME = pgcenter

SOURCE = ${PROGRAM_NAME}.go
COMMIT=$(shell git rev-parse --short HEAD)
BRANCH=$(shell git rev-parse --abbrev-ref HEAD)
TAG=$(shell git describe --tags |cut -d- -f1)

LDFLAGS = -ldflags "-X github.com/lesovsky/pgcenter/cmd.gitTag=${TAG} \
-X github.com/lesovsky/pgcenter/cmd.gitCommit=${COMMIT} \
-X github.com/lesovsky/pgcenter/cmd.gitBranch=${BRANCH}"

.PHONY: all clean install uninstall

all: pgcenter

pgcenter:
	go mod download
	CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build ${LDFLAGS} -o ${PROGRAM_NAME} ${SOURCE}

install:
	install -pm 755 ${PROGRAM_NAME} /usr/bin/${PROGRAM_NAME}

clean:
	rm -f ${PROGRAM_NAME}

uninstall:
	rm -f /usr/bin/${PROGRAM_NAME}
