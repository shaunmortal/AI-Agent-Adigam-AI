# Evaluation

## Reasoning judge

`reasoning-testset.json` holds hand-written student reasoning on real Class 10 physics questions,
each with the verdict (sound, flawed or missing) and, when flawed, the misconception label the
judge should return. `reasoning-testset-review.md` is the review of that set.
`reasoning-heldout.json` is a second set in the same format, written after the slip fix to the
judge's prompt, without looking at the prompt or earlier results, and sharing no question with the first.

Rerun the judge (reads `server/.env`; every item is one call to the real model). Pass `-out`:
the default, `eval/results/`, holds the first run and would be overwritten.

```sh
(cd server && go run ./cmd/evalreasoning -in ../eval/reasoning-testset.json -out ../eval/results/after-slip-fix)
(cd server && go run ./cmd/evalreasoning -in ../eval/reasoning-heldout.json -out ../eval/results/heldout)
```

Each item goes through `practice.JudgeReasoning`, the same call the practice route makes: the
question as the database holds it, the same prompt, schema, known labels, model tier and timeout.
The learner is a fresh one, so the known labels are the question's own option labels. A failed
call is retried once.

Each run writes two files to its `-out` folder. Three runs are kept:

| Folder | Test set | Prompt | Result |
|---|---|---|---|
| `results/` (the two `reasoning-eval*.json` files) | `reasoning-testset.json`, blind | before the slip fix | 32/36 verdicts, 18/18 labels |
| `results/after-slip-fix/` | `reasoning-testset.json`, no longer blind (the fix was written after its misses) | after | 36/36, 18/18 |
| `results/heldout/` | `reasoning-heldout.json`, held out | after | 16/16, 10/10 |

The two files:

- `reasoning-eval.json`: per item, the expected and returned verdict and misconception, the
  evidence and feedback the model gave, and the correct flags.
- `reasoning-eval-summary.json`: verdict accuracy overall, per category and per language;
  exact misconception matches (a flawed verdict with the expected label, case and spacing
  ignored) on items expected flawed; false alarms
  (flawed on `right-option-sound` and `wrong-option-sound-slip` items); and `labelMismatches`,
  the flawed items whose label did not match exactly. Whether a mismatched label still means the
  same thing is judged by hand from that list.

## Grounded tutor

`testset.json` is the test set for `server/cmd/evalgrounding` (usage at the top of that file);
`ragas_eval.py` scores its output with RAGAS.
