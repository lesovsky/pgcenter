### README: pgcenter report

`pgcenter report` is the second part of "Poor man's monitoring" which reads stats files written by pgcenter record and build reports.

- [General information](#general-information)
- [Main functions](#main-functions)
- [Usage](#usage)
---

#### General information
`pgcenter report` is an addition to pgcenter record. It reads  the collected statistics  and builds reports out of these data.

`pgcenter report` doesn't require connection to Postgres, all you need  is to specify the file with relevant statistics and choose the type of the report.

#### Main functions
- building reports from wide spectrum of Postgres stats; 
- building reports based on start and end times;
- specifying sort order based on values of specified column;
- filtering stats to show only relevant information (support regular expressions);
- limiting the amount of printed stats and showing only required information;
- showing short description of stats columns - no need to visit Postgres documentation (limited feature, will be expanded in next releases). 

#### Usage
Run `report` command to read previously written file and build a report about databases:
```
pgcenter report -f /tmp/stats.tar --database
```

See other usage examples [here](examples.md).