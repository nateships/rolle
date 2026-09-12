The Windows and Linux release tasks build the `rolle` command into this
directory before the app compiles. The app embeds whatever is here and writes
it to a user directory on **Install command**. A dev build embeds only this
file, so the app reports the command as unavailable.
