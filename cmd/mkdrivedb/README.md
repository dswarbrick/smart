# mkdrivedb

mkdrivedb is a simple tool to retrieve the drivedb.h drive "database" from [smartmontools][1], and
convert it to a more Go-friendly YAML format.

## Usage

If mkdrivedb is run without any options, it will try to download drivedb.h from the default URL,
and write our the YAML formatted drivedb to the current directory.

```
$ mkdrivedb -h
Usage of mkdrivedb:
  -in string
        Optional path to local drivedb.h
  -out string
        Output .yaml filename (default "drivedb.yaml")
  -url string
        Optional drivedb URL (default "https://www.smartmontools.org/export/HEAD/trunk/smartmontools/drivedb.h")
```

[1]: https://www.smartmontools.org/
