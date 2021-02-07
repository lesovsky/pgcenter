### Development

Download pgcenter-testing docker image and run container.
```shell
$ git pull lesovsky/pgcenter-testing
$ docker run --rm -p 21995:21995 -p 21996:21996 -p 21910:21910 -p 21911:21911 -p 21912:21912 -p 21913:21913 -ti lesovsky/pgcenter-testing:v0.0.1 /bin/bash
# prepare-test-environment.sh
```

Clone the repo.
```shell
$ git clone https://github.com/lesovsky/pgcenter
$ cd pgcenter
```

Run tests
```shell
$ make lint
$ make test
```