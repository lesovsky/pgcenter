PROGRAM_NAME = pgcenter
SOURCE = pgcenter.c
CC ?= gcc
CFLAGS = -g -std=gnu99 -Wall -pedantic
PREFIX ?= /usr
PGCONFIG ?= pg_config
PGLIBDIR = $(shell $(PGCONFIG) --libdir)
PGINCLUDEDIR = $(shell $(PGCONFIG) --includedir)
ifndef NCONFIG
	ifeq ($(shell sh -c 'which ncurses5-config>/dev/null 2>/dev/null && echo y'), y)
		NCONFIG = ncurses5-config
	else ifeq ($(shell sh -c 'which ncursesw5-config>/dev/null 2>/dev/null && echo y'), y)
		NCONFIG = ncursesw5-config
	endif
endif
NLIBS = $(shell $(NCONFIG) --libs)
NLIBDIR = $(shell $(NCONFIG) --libdir)
NINCLUDEDIR = $(shell $(NCONFIG) --includedir)
# For some distributions (at least openSUSE 13.1) ncurses5-config gives a wrong path
ifeq "$(wildcard $(NINCLUDEDIR)/menu.h )" ""
	ifneq "$(wildcard /usr/include/ncurses )" ""
		NINCLUDEDIR = /usr/include/ncurses
	endif
endif
LIBS = $(NLIBS) -lmenu -lpq
DESTDIR ?=

.PHONY: all clean install

all: pgcenter

pgcenter: pgcenter.c
	$(CC) $(CFLAGS) -I$(PGINCLUDEDIR) -L$(PGLIBDIR) -I$(NINCLUDEDIR) -L$(NLIBDIR) -o $(PROGRAM_NAME) $(SOURCE) $(LIBS)

clean:
	rm -f $(PROGRAM_NAME)

install:
	mkdir -p $(DESTDIR)$(PREFIX)/bin/
	install -pm 755 $(PROGRAM_NAME) $(DESTDIR)$(PREFIX)/bin/

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(PROGRAM_NAME)
