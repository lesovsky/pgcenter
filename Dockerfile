# can't currently use alpine because qsort_r is not provided (among other things)
FROM ubuntu

RUN apt-get update && apt-get install -y \
  build-essential \
  libpq-dev \
  libncurses-dev \
  && rm -rf /var/lib/apt/lists/*

ADD . /

RUN make && make install

ENTRYPOINT ["./pgcenter"]
