# Credits

The core logic for reading Apple Voice Memos was ported from Pedram Amini's
public-domain (CC0) gist:

https://gist.github.com/pedramamini/f4efacfe7080e07e18f54e13d8243dc1

Specifically, the following were reimplemented in Go from that script:

- Querying the `CloudRecordings.db` Core Data SQLite database.
- Resolving a recording's real audio path (`.m4a` vs `.qta`).
- Extracting Apple's on-device transcript by scanning the audio file for the
  embedded `{"attributedString":` JSON and flattening its `runs` array.

The original is dedicated to the public domain under CC0; this project is
licensed under MIT (see `LICENSE`).
