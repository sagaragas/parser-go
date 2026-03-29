# NASA Kennedy Space Center access logs

Source: [NASA-HTTP dataset](https://ita.ee.lbl.gov/html/contrib/NASA-HTTP.html) from the Internet Traffic Archive.

Two months of all HTTP requests to the NASA Kennedy Space Center web server in Florida, collected July--August 1995. The full dataset contains 3.46 million requests across both months.

## Files

- `nasa_10k.log` -- First 10,000 lines from the July 1995 log (1.1 MB). Committed to the repo for reproducible CI benchmarks.

## Full dataset

For extended benchmarks, download the complete July 1995 log (~205 MB uncompressed):

```sh
curl -o /tmp/NASA_access_log_Jul95.gz ftp://ita.ee.lbl.gov/traces/NASA_access_log_Jul95.gz
gunzip /tmp/NASA_access_log_Jul95.gz
```

Alternatively, a GitHub mirror: [greymd/NASA-HTTP](https://github.com/greymd/NASA-HTTP).

## Format

Common Log Format (a subset of Combined Log Format):

```
199.72.81.55 - - [01/Jul/1995:00:00:01 -0400] "GET /history/apollo/ HTTP/1.0" 200 6245
```

## License

The traces may be freely redistributed. See the [original page](https://ita.ee.lbl.gov/html/contrib/NASA-HTTP.html) for attribution.
