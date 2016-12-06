PROGRAM_NAME = pgcenter
SOURCE = pgcenter.c
CC ?= gcc
CFLAGS = -g -std=gnu99 -Wall -pedantic
PREFIX ?= /usr
INCLUDEDIR =
LIBDIR =
MANDIR = /usr/share/man/man1

# PostgreSQL stuff
PGCONFIG ?= pg_config
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

.PHONY: all clean install install-man uninstall

all: pgcenter

pgcenter: pgcenter.c
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(LIBS) -c common.c
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(LIBS) -c stats.c
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(LIBS) -c pgf.c
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(LIBS) -c hotkeys.c
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) $(LIBS) -o $(PROGRAM_NAME) $(SOURCE) common.o stats.o pgf.o hotkeys.o

clean:
	rm -f $(PROGRAM_NAME) common.o stats.o pgf.o hotkeys.o

install:
	mkdir -p $(DESTDIR)$(PREFIX)/bin/
	install -pm 755 $(PROGRAM_NAME) $(DESTDIR)$(PREFIX)/bin/

install-man:
	gzip -c doc/$(PROGRAM_NAME).1 > $(MANDIR)/$(PROGRAM_NAME).1.gz

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(PROGRAM_NAME)
	rm -f $(MANDIR)/$(PROGRAM_NAME).1.gz
