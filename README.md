# Roblox Lua API Archive

This repository contains an archive of previous versions of the Roblox Lua
API. The archive contains data only up to the last version before the JSON API
dump format was considered stable.

Each subdirectory of the `data` directory contains a particular type of data.
Each subdirectory within that contains the data in a particular format. Files
are named by the version hash of the build they were derived from.

	data/<type>/<format>/<version-hash>.<format>

Available types and formats are:

- `api-dump`
	- `txt`
	- `json`
- `reflection-metadata`
	- `xml`

The `builds.json` file contains a list of metadata about each build, including
version hashes.

The `latest.json` file contains metadata about the latest build (that is, the
latest in this archive).

Files in this repository can be accessed through HTTP via the following URL:

	https://raw.githubusercontent.com/RobloxAPI/archive/master/

The API dumps in JSON format have been translated from the original dump
format. Their content may change over time as they become more accurate.
