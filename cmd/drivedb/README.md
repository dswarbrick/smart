DriveDB
=======

DriveDB is a simple tool that leverages cgo to read the drivedb.h file that
ships with [smartmontools][1], writing out the database in .toml format.

The drivedb.h file is included under the terms of the Smartmontools license,
with minimal modifications in order to be compatible with Go's cgo package.

[1]: https://www.smartmontools.org/
