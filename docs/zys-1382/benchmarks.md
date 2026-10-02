# ZYS-1382 primitive benchmarks

One run of `go test -count=1 -run xxx -bench . -benchmem ./conformance/zys1382/...`
on 2026-10-01, `GOMAXPROCS=4`, Apple M1 Max, darwin/arm64, Go 1.27.1.

The host was heavily loaded by other work: load averages were
47.69 / 59.45 / 53.17 (1 / 5 / 15 min) at the start and 54.07 / 58.48 / 53.40
at the end. Treat these numbers as a loaded-machine baseline, not peak speed.
Iteration counts are low (several ran once), so expect wide run-to-run spread.

| Benchmark                       | Iterations |    Time/op |  Throughput |    Bytes/op | Allocs/op |
| ------------------------------- | ---------: | ---------: | ----------: | ----------: | --------: |
| BasisCurrentCheck_Tree2000Files |          3 |   357.5 ms |             |  20,926,773 |    38,798 |
| TreeChecksum_3000SmallFiles     |          3 |   423.9 ms |             |   5,264,261 |    55,434 |
| FileChecksum_16MiB              |         90 |    36.0 ms | 465.84 MB/s |       1,766 |        12 |
| Download_Parallel32x64KiB       |          2 |   554.1 ms |   3.79 MB/s |   4,396,048 |     6,785 |
| Exec_True                       |         88 |    15.4 ms |             |      85,165 |        73 |
| Extract_5000FileTarGz           |          1 | 3,746.6 ms |             | 185,832,048 |   285,536 |
| Extract_2000FileZip             |          1 | 2,047.6 ms |             |  74,918,912 |   112,282 |
| FileWrite_4KiBChanged           |         49 |    25.4 ms |   0.16 MB/s |      30,440 |        67 |
| FileWrite_AlreadyCurrent        |     10,000 |   0.137 ms |             |       6,353 |        15 |
| Find_WideTree                   |          5 |   235.4 ms |             |  10,492,968 |   102,646 |
| Patch_100Files                  |          1 | 1,829.6 ms |             |   1,702,560 |    15,735 |
| Patch_OneHunkIn100kLineFile     |         13 |   102.7 ms |             |  41,108,331 |       397 |
| TreeRemove_3000Files            |          2 |   924.9 ms |             |     253,980 |     6,579 |
| TreeWrite_2000Files             |          1 | 1,279.8 ms |             |  74,502,632 |   114,370 |

The `integration` package has no benchmarks.

Reproduce:

```sh
GOCACHE=$HOME/Library/Caches/go-build GOMAXPROCS=4 \
  go test -count=1 -run xxx -bench . -benchmem ./conformance/zys1382/...
```
