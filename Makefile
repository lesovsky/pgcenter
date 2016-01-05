PROGRAM_NAME = pgcenter
SOURCE = pgcenter.c
CC ?= gcc
CFLAGS = -g -std=gnu99 -Wall -pedantic
PREFIX ?= /usr
INCLUDEDIR =
LIBDIR =

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
NLIBDIR = $(shell $(NCONFIG) --libdir)
NINCLUDEDIR = $(shell $(NCONFIG) --includedir)
NLIBS = $(shell $(NCONFIG) --libs) -lmenu
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

.PHONY: all clean install

all: pgcenter

pgcenter: pgcenter.c
	$(CC) $(CFLAGS) $(INCLUDEDIR) $(LIBDIR) -o $(PROGRAM_NAME) $(SOURCE) $(LIBS)

clean:
	rm -f $(PROGRAM_NAME)

install:
	mkdir -p $(DESTDIR)$(PREFIX)/bin/
	install -pm 755 $(PROGRAM_NAME) $(DESTDIR)$(PREFIX)/bin/

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(PROGRAM_NAME)
