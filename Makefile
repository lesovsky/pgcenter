PROGRAM_NAME = pgcenter
SOURCES = ./src/*.c
CC ?= gcc
CFLAGS = -std=gnu99 -pedantic -Wall -Wextra -Wfloat-equal
CFLAGS_DEV = -g
PREFIX ?= /usr
INCLUDEDIR =
LIBDIR =
SHAREDIR = $(PREFIX)/share
MANDIR = $(SHAREDIR)/man/man1

# PostgreSQL stuff
RHEL_PGPATH = $(shell find /usr -maxdepth 1 -type d -name "pgsql-*" | sort -V |tail -n 1)
RHEL_PGBINDIR = $(RHEL_PGPATH)/bin
PATH := $(PATH):$(RHEL_PGBINDIR)
PGCONFIG ?= $(shell env PATH=$(PATH) which pg_config)
PGLIBDIR = $(shell $(PGCONFIG) --libdir)
PGINCLUDEDIR = $(shell $(PGCONFIG) --includedir)
PGLIBS = -lpq
ifneq ($(PGLIBDIR),)
        LIBDIR += -L$(PGLIBDIR)
endif
ifneq ($(PGINCLUDEDIR),)
        INCLUDEDIR += -I$(PGINCLUDEDIR)
endif

# Ncurses stuff
ifndef NCONFIG
        ifeq ($(shell sh -c 'which ncurses5-config>/dev/null 2>/dev/null && echo y'), y)
                NCONFIG = ncurses5-config
        else ifeq ($(shell sh -c 'which ncursesw5-config>/dev/null 2>/dev/null && echo y'), y)
                NCONFIG = ncursesw5-config
        else ifeq ($(shell sh -c 'which ncurses6-config>/dev/null 2>/dev/null && echo y'), y)
                NCONFIG = ncurses6-config
        else ifeq ($(shell sh -c 'which ncursesw6-config>/dev/null 2>/dev/null && echo y'), y)
                NCONFIG = ncursesw6-config
        endif
endif

ifdef NCONFIG
    NLIBDIR = $(shell $(NCONFIG) --libdir)
    NINCLUDEDIR = $(shell $(NCONFIG) --includedir)
    NLIBS = $(shell $(NCONFIG) --libs) -lmenu
else
        NLIBS = -lncurses -lmenu
endif

ifneq ($(NLIBDIR),)
        LIBDIR += -L$(NLIBDIR)
endif
ifneq ($(NINCLUDEDIR),)
        ifeq "$(wildcard $(NINCLUDEDIR)/menu.h )" ""
                ifneq "$(wildcard $(NINCLUDEDIR)/ncurses )" ""
                        INCLUDEDIR += -I$(NINCLUDEDIR)/ncurses
                endif
        endif
endif

# General stuff
LIBS = $(PGLIBS) $(NLIBS)
DESTDIR ?=

.PHONY: all devel clean install install-man uninstall

all: pgcenter

pgcenter:
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(SOURCES) $(LIBS) -o $(PROGRAM_NAME)

devel:
	$(CC) $(CFLAGS_DEV) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(SOURCES) $(LIBS) -o $(PROGRAM_NAME)

clean:
	rm -f $(PROGRAM_NAME)

install:
	mkdir -p $(DESTDIR)$(PREFIX)/bin/
	mkdir -p $(DESTDIR)$(SHAREDIR)/$(PROGRAM_NAME)/
	install -pm 755 $(PROGRAM_NAME) $(DESTDIR)$(PREFIX)/bin/
	install -pm 644 share/init-stats-schema-plperlu.sql $(DESTDIR)$(SHAREDIR)/$(PROGRAM_NAME)/
	install -pm 644 share/init-stats-views.sql $(DESTDIR)$(SHAREDIR)/$(PROGRAM_NAME)/

install-man:
	gzip -c share/doc/$(PROGRAM_NAME).1 > $(MANDIR)/$(PROGRAM_NAME).1.gz

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(PROGRAM_NAME)
	rm -f $(DESTDIR)$(MANDIR)/$(PROGRAM_NAME).1.gz
	rm -f $(DESTDIR)$(SHAREDIR)/$(PROGRAM_NAME)/init-stat-functions-plperlu.sql
	rmdir $(DESTDIR)$(SHAREDIR)/$(PROGRAM_NAME)/
