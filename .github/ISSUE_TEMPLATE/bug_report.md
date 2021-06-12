---
name: Bug report
about: Create a report to help improve pgCenter
title: ''
labels: ''
assignees: ''

---

**Describe the bug**
A clear and concise description of what the bug is.

**Environment**
Describe the environment where the bug occurred.
- OS: [output of `cat /etc/os-release`]
- Docker image name and tag (in case of Docker-related environment)  
- pgCenter Version [output of `pgcenter --version`]
- pgCenter installation method: [releases page, manual build, other?]
- PostgreSQL Version [output of `psql -c 'select version()'`]
- Did you try build pgCenter from master branch and reproduce the issue? ['yes' or 'no']

**To Reproduce**
Describe the steps to reproduce the behavior. Attach the full error text, panic stack trace, screenshots, etc. 

**Expected behavior**
A clear and concise description of what you expected to happen.

**Additional context**
Add any other context about the problem here. This could be:
- PostgreSQL error logs
- Your assumptions, thoughts, hypothesis, etc 
- Whatever else?
