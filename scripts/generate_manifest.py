#!/usr/bin/env python3
"""
VXR-Thesis Commit Manifest Generator
Extracts git commit metadata and maintains a structured JSON manifest log.
Used by the Commit Manifest Bot in CI/CD and available for local execution.
"""

import argparse
import datetime
import json
import os
import re
import subprocess
import sys


def run_git(args, cwd=None):
    """Run a git command and return its stdout as stripped text."""
    cmd = ["git"] + args
    result = subprocess.run(
        cmd,
        cwd=cwd,
        capture_output=True,
        text=True,
        check=True,
        encoding="utf-8",
        errors="replace",
    )
    return result.stdout.strip()


def parse_shortstat(stat_text):
    """Parse shortstat string like '3 files changed, 10 insertions(+), 2 deletions(-)'."""
    stats = {"files_changed": 0, "insertions": 0, "deletions": 0}
    if not stat_text:
        return stats

    m_files = re.search(r"(\d+)\s+files?\s+changed", stat_text)
    if m_files:
        stats["files_changed"] = int(m_files.group(1))

    m_ins = re.search(r"(\d+)\s+insertions?", stat_text)
    if m_ins:
        stats["insertions"] = int(m_ins.group(1))

    m_del = re.search(r"(\d+)\s+deletions?", stat_text)
    if m_del:
        stats["deletions"] = int(m_del.group(1))

    return stats


def get_commit_files(sha):
    """Get list of changed files and their status for a given commit."""
    files = []
    try:
        raw_output = run_git(["diff-tree", "--no-commit-id", "--name-status", "-r", "--root", sha])
        for line in raw_output.splitlines():
            line = line.strip()
            if not line:
                continue
            parts = line.split("\t")
            if len(parts) >= 2:
                status = parts[0]
                path = parts[1]
                files.append({"status": status, "path": path})
    except subprocess.CalledProcessError as e:
        print(f"[WARN] Failed to get diff-tree for {sha}: {e}", file=sys.stderr)
    return files


def extract_commit_data(sha, repo_name="Voxion-Labs/VXR-Thesis", ref="refs/heads/main", actor="github-actions[bot]"):
    """Extract comprehensive metadata for a specific commit."""
    full_sha = run_git(["rev-parse", sha])
    short_sha = run_git(["rev-parse", "--short", full_sha])
    tree_sha = run_git(["log", "-1", "--format=%T", full_sha])
    parents_raw = run_git(["log", "-1", "--format=%P", full_sha])
    parents = parents_raw.split() if parents_raw else []

    author_name = run_git(["log", "-1", "--format=%an", full_sha])
    author_email = run_git(["log", "-1", "--format=%ae", full_sha])
    author_date = run_git(["log", "-1", "--format=%aI", full_sha])

    committer_name = run_git(["log", "-1", "--format=%cn", full_sha])
    committer_email = run_git(["log", "-1", "--format=%ce", full_sha])
    committer_date = run_git(["log", "-1", "--format=%cI", full_sha])

    subject = run_git(["log", "-1", "--format=%s", full_sha])
    body = run_git(["log", "-1", "--format=%b", full_sha])
    message = run_git(["log", "-1", "--format=%B", full_sha])

    try:
        shortstat_raw = run_git(["show", "--shortstat", "--format=", full_sha])
        stats = parse_shortstat(shortstat_raw)
    except subprocess.CalledProcessError:
        stats = {"files_changed": 0, "insertions": 0, "deletions": 0}

    files = get_commit_files(full_sha)
    if stats["files_changed"] == 0 and files:
        stats["files_changed"] = len(files)

    now_iso = datetime.datetime.now(datetime.timezone.utc).isoformat()

    return {
        "sha": full_sha,
        "short_sha": short_sha,
        "tree": tree_sha,
        "parents": parents,
        "author": {
            "name": author_name,
            "email": author_email,
            "date": author_date,
        },
        "committer": {
            "name": committer_name,
            "email": committer_email,
            "date": committer_date,
        },
        "subject": subject,
        "body": body,
        "message": message,
        "branch": ref.replace("refs/heads/", ""),
        "repository": repo_name,
        "url": f"https://github.com/{repo_name}/commit/{full_sha}",
        "stats": stats,
        "files_count": len(files),
        "files": files,
        "bot_manifest": {
            "recorded_by": actor,
            "recorded_at": now_iso,
            "generator_version": "1.0.0",
        },
    }


def record_commit(sha="HEAD", output_dir="manifests", backfill=False):
    """Record commit data into JSON files and update manifest.json."""
    os.makedirs(output_dir, exist_ok=True)
    commits_dir = os.path.join(output_dir, "commits")
    os.makedirs(commits_dir, exist_ok=True)
    manifest_file = os.path.join(output_dir, "manifest.json")

    repo_name = os.environ.get("GITHUB_REPOSITORY", "Voxion-Labs/VXR-Thesis")
    ref = os.environ.get("GITHUB_REF", "refs/heads/main")
    actor = os.environ.get("GITHUB_ACTOR", "github-actions[bot]")

    # Load existing manifest if present
    manifest_data = {
        "schema_version": "1.0.0",
        "repository": repo_name,
        "branch": ref.replace("refs/heads/", ""),
        "last_updated": None,
        "total_commits": 0,
        "latest_sha": None,
        "commits": [],
    }

    if os.path.exists(manifest_file):
        try:
            with open(manifest_file, "r", encoding="utf-8") as f:
                loaded = json.load(f)
                if isinstance(loaded, dict) and "commits" in loaded:
                    manifest_data.update(loaded)
        except Exception as e:
            print(f"[WARN] Could not parse existing manifest: {e}", file=sys.stderr)

    existing_commits_map = {c["sha"]: c for c in manifest_data.get("commits", []) if "sha" in c}

    shas_to_process = []
    if backfill:
        # Get all commit SHAs in reverse chronological order
        try:
            all_shas = run_git(["rev-list", "--reverse", "HEAD"]).splitlines()
            shas_to_process.extend([s.strip() for s in all_shas if s.strip()])
        except subprocess.CalledProcessError as e:
            print(f"[ERROR] Failed to list rev-list: {e}", file=sys.stderr)
            shas_to_process.append(sha)
    else:
        resolved_sha = run_git(["rev-parse", sha])
        shas_to_process.append(resolved_sha)

    print(f"[INFO] Processing {len(shas_to_process)} commit(s)...")

    for target_sha in shas_to_process:
        commit_info = extract_commit_data(target_sha, repo_name=repo_name, ref=ref, actor=actor)
        full_sha = commit_info["sha"]

        # Save single commit json
        commit_path = os.path.join(commits_dir, f"{full_sha}.json")
        with open(commit_path, "w", encoding="utf-8") as f:
            json.dump(commit_info, f, indent=2, ensure_ascii=False)
        print(f"  -> Saved commit metadata to {commit_path}")

        # Update summary for manifest index
        summary_entry = {
            "sha": commit_info["sha"],
            "short_sha": commit_info["short_sha"],
            "date": commit_info["author"]["date"],
            "author": commit_info["author"]["name"],
            "subject": commit_info["subject"],
            "files_changed": commit_info["stats"]["files_changed"],
            "insertions": commit_info["stats"]["insertions"],
            "deletions": commit_info["stats"]["deletions"],
            "manifest_file": f"manifests/commits/{full_sha}.json",
        }
        existing_commits_map[full_sha] = summary_entry

    # Sort commits chronologically descending (newest first)
    all_summaries = list(existing_commits_map.values())
    all_summaries.sort(key=lambda x: x.get("date", ""), reverse=True)

    now_iso = datetime.datetime.now(datetime.timezone.utc).isoformat()
    manifest_data["last_updated"] = now_iso
    manifest_data["total_commits"] = len(all_summaries)
    manifest_data["latest_sha"] = all_summaries[0]["sha"] if all_summaries else None
    manifest_data["commits"] = all_summaries

    with open(manifest_file, "w", encoding="utf-8") as f:
        json.dump(manifest_data, f, indent=2, ensure_ascii=False)
    print(f"[INFO] Updated master manifest: {manifest_file} (Total: {manifest_data['total_commits']} commits)")


def main():
    parser = argparse.ArgumentParser(description="Generate commit JSON metadata manifest.")
    parser.add_argument("--commit-sha", default="HEAD", help="Commit SHA or reference to process (default: HEAD)")
    parser.add_argument("--output-dir", default="manifests", help="Output directory for manifests (default: manifests)")
    parser.add_argument("--backfill", action="store_true", help="Backfill all historical commits in repository")
    args = parser.parse_args()

    record_commit(sha=args.commit_sha, output_dir=args.output_dir, backfill=args.backfill)


if __name__ == "__main__":
    main()
