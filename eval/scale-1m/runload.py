"""Runs the scale1m load test: machine + Postgres settings once, a warm-up, then one fixed-duration
scaleload run per concurrency, with load average, top processes and Postgres counters around each.
Usage: python3 -I runload.py [--label-prefix X] [--duration 60s] [--conc 32,64,128,256] [--warmup 30s]
Writes results/<prefix>c<N>.json (load generator) and results/<prefix>c<N>.meta.json (context)."""
import argparse, json, os, subprocess, threading, time

S = os.path.dirname(os.path.abspath(__file__))
R = os.path.join(S, "results")
PSQL = ["/Library/PostgreSQL/18/bin/psql", "-h", "localhost", "-p", "5544", "-U", "postgres", "-At", "-X"]
LOAD = [os.path.join(S, "scaleload", "scaleload"), "-questions", os.path.join(S, "scaleload", "questions.csv")]


def sh(*cmd):
    return subprocess.run(cmd, capture_output=True, text=True, check=True).stdout.strip()


def sql(q):
    return sh(*PSQL, "-c", q)


def sql_json(q):
    return json.loads(sql(f"select coalesce(json_agg(t), '[]') from ({q}) t"))


def pg_counters():
    row = sql_json("""select xact_commit, xact_rollback, blks_read, blks_hit, tup_inserted, tup_updated,
        tup_fetched, deadlocks, temp_files, temp_bytes, blk_read_time, blk_write_time
        from pg_stat_database where datname = 'postgres'""")[0]
    row["question_attempts"] = int(sql("select count(*) from question_attempts"))
    row["db_bytes"] = int(sql("select pg_database_size('postgres')"))
    return row


def file_lines(path):
    with open(path, errors="replace") as f:
        return f.readlines()


def context(phase):
    return {
        "phase": phase, "at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "loadavg_1_5_15": sh("sysctl", "-n", "vm.loadavg"),
        "top_cpu_processes": sh("sh", "-c", "ps -Ao pcpu,pmem,comm -r | head -11").splitlines(),
        "memory_pressure": sh("sh", "-c", "memory_pressure -Q 2>/dev/null || true"),
        "swap": sh("sysctl", "-n", "vm.swapusage"),
        "disk_free": sh("sh", "-c", f"df -h {S} | tail -1"),
        "pg": pg_counters(),
        "api_log_lines": len(file_lines(os.path.join(S, "api.log"))),
        "pg_log_lines": len(file_lines(os.path.join(S, "pg.log"))),
    }


def log_delta(path, start):
    """Counts new log lines by kind: API slog level + message, Postgres slow statements/errors."""
    lines = file_lines(path)[start:]
    kinds, samples = {}, []
    for l in lines:
        words = l.split()
        key = None
        for lvl in ("WARN", "ERROR"):
            if lvl in words[:4]:
                i = words.index(lvl)
                msg = [w for w in words[i + 1:] if "=" not in w][:8]
                key = lvl + " " + " ".join(msg)
        if "duration:" in l:
            key = "postgres statement over log_min_duration_statement (500 ms)"
        elif "ERROR:" in l or "FATAL:" in l:
            key = "postgres " + l.split("ERROR:" if "ERROR:" in l else "FATAL:")[1].strip()[:80]
        if key:
            kinds[key] = kinds.get(key, 0) + 1
            if ("ERROR" in key or "duration:" in l) and len(samples) < 8:
                samples.append(l.rstrip()[:400])
    return {"new_lines": len(lines), "kinds": kinds, "samples": samples}


def once():
    machine = {
        "hw.model": sh("sysctl", "-n", "hw.model"), "cpu": sh("sysctl", "-n", "machdep.cpu.brand_string"),
        "hw.ncpu": int(sh("sysctl", "-n", "hw.ncpu")),
        "performance_cores": int(sh("sysctl", "-n", "hw.perflevel0.physicalcpu")),
        "efficiency_cores": int(sh("sysctl", "-n", "hw.perflevel1.physicalcpu")),
        "hw.memsize_bytes": int(sh("sysctl", "-n", "hw.memsize")),
        "os": sh("sw_vers", "-productVersion") + " (" + sh("sw_vers", "-buildVersion") + ")",
        "go": sh("go", "version"), "postgres": sql("select version()"),
        "redis": sh("redis-server", "--version"),
    }
    settings = sql_json("""select name, setting, unit, source from pg_settings
        where source not in ('default', 'override') or name in ('shared_buffers', 'work_mem',
        'effective_cache_size', 'max_connections', 'synchronous_commit', 'fsync', 'wal_level',
        'random_page_cost', 'jit', 'max_worker_processes', 'max_parallel_workers_per_gather') order by name""")
    api = {"pid": int(open(os.path.join(S, "api.pid")).read()),
           "env": sh("sh", "-c", f"ps eww -o command= -p $(cat {S}/api.pid) | tr ' ' '\\n' | grep -E '^(PORT|DATABASE_URL|REDIS_URL|SUPABASE_URL|MAX_IN_FLIGHT|ANTHROPIC_API_KEY|GEMINI_API_KEY)=' || true").splitlines()}
    return {"machine": machine, "postgres_settings": settings, "api": api}


OURS = {"postgres": "postgres", "adigam-api": "api", "scaleload": "loadgen", "stubauth-bin": "stubauth",
        "redis-server": "redis"}


def sample_cpu(stop, into):
    """Every 5 s during a run: 1-min load average, CPU % of this stack's processes, top other ones."""
    while not stop.wait(5):
        mine, other = {}, {}
        pgmaster = open(os.path.join(S, "pg", "postmaster.pid")).readline().strip()
        for line in sh("ps", "-Ao", "pcpu=,pid=,ppid=,comm=").splitlines():
            pct, pid, ppid, comm = line.split(None, 3)
            name = os.path.basename(comm.strip().split(" ")[0]).rstrip(":")
            if name == "postgres":  # only this cluster's postmaster and its children are ours
                name = "postgres" if pgmaster in (pid, ppid) else "postgres (other cluster)"
            elif name == "redis-server":
                name = "redis-server" if "127.0.0.1:6390" in comm else "redis-server (other instance)"
            bucket = mine if name in OURS else other
            key = OURS.get(name, name[:40])
            bucket[key] = round(bucket.get(key, 0) + float(pct), 1)
        top = dict(sorted(other.items(), key=lambda kv: -kv[1])[:5])
        into.append({"t": time.strftime("%H:%M:%S"), "loadavg": sh("sysctl", "-n", "vm.loadavg"),
                     "stack_cpu_pct": mine, "other_cpu_pct_total": round(sum(other.values()), 1),
                     "top_other_cpu_pct": top})


def run(label, conc, duration):
    before = context("before")
    out = os.path.join(R, label + ".json")
    during, stop = [], threading.Event()
    sampler = threading.Thread(target=sample_cpu, args=(stop, during))
    sampler.start()
    line = sh(*LOAD, "-c", str(conc), "-d", duration, "-label", label, "-out", out)
    stop.set()
    sampler.join()
    after = context("after")
    meta = {"label": label, "concurrency": conc, "duration": duration, "loadgen_line": line,
            "before": before, "during": during, "after": after,
            "pg_delta": {k: after["pg"][k] - before["pg"][k] for k in before["pg"]},
            "api_log": log_delta(os.path.join(S, "api.log"), before["api_log_lines"]),
            "pg_log": log_delta(os.path.join(S, "pg.log"), before["pg_log_lines"])}
    with open(os.path.join(R, label + ".meta.json"), "w") as f:
        json.dump(meta, f, indent=2)
    print(line, "| loadavg before", before["loadavg_1_5_15"], flush=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--label-prefix", default="")
    ap.add_argument("--duration", default="60s")
    ap.add_argument("--conc", default="32,64,128,256")
    ap.add_argument("--warmup", default="30s")
    ap.add_argument("--settle", type=int, default=10)
    a = ap.parse_args()
    os.makedirs(R, exist_ok=True)
    with open(os.path.join(R, a.label_prefix + "environment.json"), "w") as f:
        json.dump(once(), f, indent=2)
    if a.warmup != "0":
        run(a.label_prefix + "warmup", 64, a.warmup)
    sql("select pg_stat_statements_reset()")  # hottest-query ranking covers the measured runs only
    for c in [int(x) for x in a.conc.split(",")]:
        time.sleep(a.settle)
        run(f"{a.label_prefix}c{c}", c, a.duration)


if __name__ == "__main__":
    main()
