# _template — start here

The canonical scaffold for a Verdant Oasis C2D sandbox algorithm. Copy it, rename,
and edit `compute()` in `train.py`. As shipped it computes safe per-column summary
stats, so it runs out of the box.

```
cp -r algorithms/_template algorithms/myalgo
# edit algorithms/myalgo/train.py  → compute()
docker build -t vo-myalgo:1 algorithms/myalgo
# run it through the real contract (no network, read-only data):
docker run --rm --network=none --read-only --tmpfs=/tmp:rw,size=64m \
  -v "$PWD/data":/data:ro -v "$PWD/out":/out vo-myalgo:1
```

Keep the security properties documented at the top of `train.py` (aggregates only,
no raw data in logs, JSON not pickle, deterministic) or it will fail review.
See `../logreg` and `../kmeans` for complete worked examples. Author it with
**Lumen** — see the `/build` page in the app.
