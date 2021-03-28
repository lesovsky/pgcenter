### Development

Download pgcenter-testing docker image, run container and start script which prepares testing environment.
```shell
$ git pull lesovsky/pgcenter-testing:latest
$ docker run --rm -p 21995:21995 -p 21996:21996 -p 21910:21910 -p 21911:21911 -p 21912:21912 -p 21913:21913 -ti lesovsky/pgcenter-testing:latest /bin/bash
# prepare-test-environment.sh
```

Clone the repo.
```shell
$ git clone https://github.com/lesovsky/pgcenter
$ cd pgcenter
```

Before running tests or building make sure your go version is 1.16.

Run tests.
```shell
$ make lint
$ make test
```

Build.
```shell
$ make build
```