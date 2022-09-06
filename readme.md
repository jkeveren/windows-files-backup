# Generic backup utility
Backs up files. Created for Windows. Use `rsync` if you're using a sensible OS.

## Features
- Stores backups in a zip archive.
- Emails on error.
- Deletes old backups unless errors occur (keeps latest 3).

## Usage
`<path to executable> <config and destination directory>`

## Config and Desintation Directory
Contents:
- `backups`: Created automatically. Directory containing the backups. Do not change filenames! They contain machine readable timestamps. Backups older than the last 3 are deleted. Changing filenames may cause your latest backup to be deleted. Modtimes are not used in case they get touched by nonesense backup utilities like OneDrive or Google Drive. They shouldn't but I dont care to find out or rely on them.
- `log.txt`: Created automatically. Logs from latest run.
- `config.json`: Configuration for the backup. Example below (remove comments, json does not support them because it is bad).
	```json
	{
		"name": "test", // Name of the backup. This will be used in any error report emails (Useful for backups on multiple machines).
		"errorContacts": [ // Contacts to email when an error occurs.
			{
				"name": "James Keveren",
				"email": "james@keve.ren"
			},
			{
				"name": "Name",
				"email": "example@example.com"
			}
		],
		"sendGridAPIKey": "YOUR_SENDGRID_API_KEY",
		"useSendGrid": true, // flag to enable sending error reports with SendGrid.
		"salesScribeAPIKey": "YOUR_SALESSCRIBE_API_KEY",
		"useSalesScribe": true, // flag to enable sending error reports with SalesScribe.
		"sources": [ // Paths to back up.
			{
				"path": "../src", // Path to back up (relative to the config directory).
				"blacklist": [ // Files not to back up.
					"*.bad",
					"blacklisted-dir"
				]
			},
			{
				"path": "../src2",
				"blacklist": [
					"*.not-good"
				]
			}
		]
	}
	```

## Testing
1. Create the `./test/dst` directory. This directory is not tracked by git.
1. Create the `./test/dst/config.json` file and populate with your config. Config is documented in the previous section.
1. Run `./test.bat` (Should also be compatable with `sh`/`bash`).
