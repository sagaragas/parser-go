#!/usr/bin/env python3

import argparse
import datetime
import json
import os
import re
import shutil
import sys
import tempfile


CONFIG_TEMPLATE = """[format]
log-pattern=(\\S+)\\s+\\S+\\s+\\S+\\s+\\[([^ ]+) [^\\]]+\\]\\s+"(\\w+)\\s+(\\S+)\\s+([^"]+)"\\s+(\\d+)\\s+\\S+
log-format=ip datetime method url protocol status

[filter]
support_method=POST,GET
is_with_parameters=0
always_parameter_keys=action
urls_most_number=200
urls_pv_threshold=1
urls_pv_threshold_time=600
urls_pv_threshold_min=1
ignore_url_suffix=.json
fixed_parameter_keys=action
custom_parameters=t={timeStamp}
ignore_urls=/health,/healthz,/readyz,/ping,/alive,/_health
static-file=css,CSS,dae,DAE,eot,EOT,gif,GIF,ico,ICO,jpeg,JPEG,jpg,JPG,js,JS,map,MAP,mp3,MP3,pdf,PDF,png,PNG,svg,SVG,swf,SWF,ttf,TTF,txt,TXT,woff,WOFF

[report]
language=english
second_line_flag=0
cost_time_percentile_flag=0
cost_time_flag=0
cost_time_threshold=0.500
upload_flag=0
upload_url=http://127.0.0.1/disabled

[goaccess]
goaccess_flag=0
time-format=%H:%M:%S
date-format=%d/%b/%Y
goaccess-log-format=%h %^[%d:%t %^] "%r" %s %b "%R" "%u"
"""


def parse_args():
    parser = argparse.ArgumentParser(description="Run the legacy Python baseline and emit canonical benchmark output.")
    parser.add_argument("--legacy-repo", required=True)
    parser.add_argument("--corpus", required=True)
    parser.add_argument("--out", required=True)
    return parser.parse_args()


def write_config(path):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(CONFIG_TEMPLATE)


def setup_workspace(legacy_repo, corpus_path):
    workspace = tempfile.mkdtemp(prefix="parsergo-legacy-bench-")
    bin_dir = os.path.join(workspace, "bin")
    conf_dir = os.path.join(workspace, "conf")
    data_dir = os.path.join(workspace, "data")
    os.makedirs(bin_dir, exist_ok=True)
    os.makedirs(conf_dir, exist_ok=True)
    os.makedirs(data_dir, exist_ok=True)
    os.symlink(os.path.join(legacy_repo, "bin", "templates"), os.path.join(bin_dir, "templates"))

    corpus_name = os.path.basename(corpus_path)
    shutil.copyfile(corpus_path, os.path.join(data_dir, corpus_name))
    write_config(os.path.join(conf_dir, "config.ini"))
    return workspace, corpus_name


def load_legacy_module(legacy_repo, workspace):
    legacy_bin = os.path.join(legacy_repo, "bin")
    if legacy_bin not in sys.path:
        sys.path.insert(0, legacy_bin)

    cwd = os.getcwd()
    os.chdir(os.path.join(workspace, "bin"))
    try:
        import start  # type: ignore
    finally:
        os.chdir(cwd)
    return start


def run_legacy_parse(start, workspace, corpus_name):
    captured = {}
    cwd = os.getcwd()
    os.chdir(os.path.join(workspace, "bin"))
    try:
        def capture(data):
            captured["data"] = data

        start.generate_web_log_parser_report = capture
        log_format = start.parse_log_format()
        start.parse_log_file(corpus_name, log_format)
    finally:
        os.chdir(cwd)

    if "data" not in captured:
        raise RuntimeError("legacy baseline did not emit report data")
    return captured["data"], log_format


def canonicalize_output(start, log_format, corpus_path):
    pattern = re.compile(start.config.log_pattern)
    counts = {}
    timestamps = []
    total_lines = 0
    matched_lines = 0
    filtered_lines = 0
    rejected_lines = 0

    with open(corpus_path, "r", encoding="utf-8") as handle:
        for raw_line in handle:
            total_lines += 1
            line = raw_line.rstrip("\n")
            match = pattern.match(line)
            if match is None:
                rejected_lines += 1
                continue

            method = match.group(log_format.get("method_index"))
            url = start.get_url(match, log_format)

            if start.is_ignore_url(url) or (not start.not_static_file(url)) or method not in start.config.support_method:
                filtered_lines += 1
                continue

            matched_lines += 1
            counts[(method, url)] = counts.get((method, url), 0) + 1
            timestamps.append(datetime.datetime.strptime(match.group(log_format.get("time_index")), "%d/%b/%Y:%H:%M:%S"))

    ranked_requests = []
    for (method, path), count in counts.items():
        percentage = 0.0
        if matched_lines:
            percentage = float(count * 100) / float(matched_lines)
        ranked_requests.append(
            {
                "path": path,
                "method": method,
                "count": count,
                "percentage": percentage,
            }
        )

    ranked_requests.sort(key=lambda item: (-item["count"], item["path"], item["method"]))

    requests_per_sec = 0.0
    if len(timestamps) >= 2:
        duration = (timestamps[-1] - timestamps[0]).total_seconds()
        if duration > 0:
            requests_per_sec = float(matched_lines) / duration

    return {
        "summary": {
            "requests_total": matched_lines,
            "requests_per_sec": requests_per_sec,
            "ranked_requests": ranked_requests,
        },
        "workload": {
            "input_bytes": os.path.getsize(corpus_path),
            "total_lines": total_lines,
            "matched_lines": matched_lines,
            "filtered_lines": filtered_lines,
            "rejected_lines": rejected_lines,
            "row_count": matched_lines,
        },
    }


def main():
    args = parse_args()
    workspace = None
    try:
        workspace, corpus_name = setup_workspace(args.legacy_repo, args.corpus)
        start = load_legacy_module(args.legacy_repo, workspace)
        captured, log_format = run_legacy_parse(start, workspace, corpus_name)
        output = canonicalize_output(start, log_format, args.corpus)

        if int(captured.get("pv", 0)) != output["workload"]["matched_lines"]:
            raise RuntimeError("legacy baseline matched-line count does not match normalized workload accounting")

        os.makedirs(os.path.dirname(args.out), exist_ok=True)
        with open(args.out, "w", encoding="utf-8") as handle:
            json.dump(output, handle, indent=2)
            handle.write("\n")
    finally:
        if workspace and os.path.isdir(workspace):
            shutil.rmtree(workspace)


if __name__ == "__main__":
    main()
