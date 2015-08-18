PROGRAM_NAME = pgcenter
SOURCE = pgcenter.c
CC ?= gcc
CFLAGS = -std=gnu99 -O2 -Wall -pedantic
PREFIX ?= /usr
PGCONFIG ?= pg_config
PGLIBDIR = $(shell $(PGCONFIG) --libdir)
PGINCLUDEDIR = $(shell $(PGCONFIG) --includedir)
NCONFIG ?= ncurses5-config
NLIBS = $(shell $(NCONFIG) --libs)
LIBS = $(NLIBS) -lpq
DESTDIR ?=

.PHONY: all clean install

all: pgcenter

pgcenter: pgcenter.c
	gcc $(CFLAGS) -I$(PGINCLUDEDIR) -L$(PGLIBDIR) -o $(PROGRAM_NAME) $(SOURCE) $(LIBS)

clean:
	rm -f $(PROGRAM_NAME)

install:
	mkdir -p $(DESTDIR)$(PREFIX)/bin/
	install -pm 755 $(PROGRAM_NAME) $(DESTDIR)$(PREFIX)/bin/

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(PROGRAM_NAME)
