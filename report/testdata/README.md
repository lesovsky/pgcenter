### Testing notes

#### How to create golden report with valid order of archived files.
```
tar cf pgcenter.stat.golden.tar $(for ts in $(ls |cut -d. -f2 |sort -u); do ls meta.$ts.123.json; ls *.$ts.123.json |grep -v meta; done |xargs)
```