# fixtures/observed — real LIMIT observations

These are real observations, not hand-written fixtures: the loop (`archive_limit` in `lib.sh`) copies every LIMIT-classified sitting's final result line (`result.json`), its `status.json`, its `system/api_retry` events (`api_retry.jsonl`) and the classification (`class.txt`) into `<sitting ts>/` here. Promote them into `test-classify.sh` cases so the classifier is tested against what the CLI actually emits, then delete the promoted directory.
