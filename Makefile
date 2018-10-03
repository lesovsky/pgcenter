PROGRAM_NAME = pgcenter
PREFIX ?= /usr
INCLUDEDIR =
LIBDIR =
SHAREDIR = ${PREFIX}/share
MANDIR = ${SHAREDIR}/man/man1

SOURCE = ${PROGRAM_NAME}.go
COMMIT=$(shell git rev-parse HEAD)
BRANCH=$(shell git rev-parse --abbrev-ref HEAD)

LDFLAGS = -ldflags "-X main.COMMIT=${COMMIT} -X main.BRANCH=${BRANCH}"

DESTDIR ?=

.PHONY: all clean install uninstall

all: pgcenter

pgcenter:
	go mod download
	GOOS=linux GOARCH=${GOARCH} go build ${LDFLAGS} -o ${PROGRAM_NAME} ${SOURCE}

install:
	mkdir -p ${DESTDIR}${PREFIX}/bin/
	install -pm 755 ${PROGRAM_NAME} ${DESTDIR}${PREFIX}/bin/${PROGRAM_NAME}

clean:
	rm -f ${PROGRAM_NAME}

uninstall:
	rm -f ${DESTDIR}${PREFIX}/bin/${PROGRAM_NAME}
