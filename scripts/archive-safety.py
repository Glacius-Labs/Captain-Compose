#!/usr/bin/env python3
"""Validate and safely stage Captain Compose backup archives."""
import os
import pathlib
import shutil
import sys
import tarfile

ALLOWED = ("var/lib/captain-compose", "etc/captain-compose")


def members(archive):
    tf = tarfile.open(archive, "r:gz")
    seen = set()
    valid = []
    roots = set()
    total = 0
    for item in tf.getmembers():
        name = item.name.removeprefix("./")
        path = pathlib.PurePosixPath(name)
        if path.is_absolute() or not name or any(part in ("", ".", "..") for part in path.parts):
            raise ValueError(f"unsafe archive member path: {item.name!r}")
        if not any(name == root or name.startswith(root + "/") for root in ALLOWED):
            raise ValueError(f"unexpected archive path: {name!r}")
        if name in seen:
            raise ValueError(f"duplicate archive path: {name!r}")
        seen.add(name)
        if not (item.isdir() or item.isfile()):
            raise ValueError(f"unsupported link or special file: {name!r}")
        total += item.size
        if total > 100 * 1024 * 1024 * 1024:
            raise ValueError("archive exceeds the 100 GiB safety limit")
        roots.add(ALLOWED[0] if name.startswith(ALLOWED[0]) else ALLOWED[1])
        valid.append((item, name))
    if roots != set(ALLOWED) or "etc/captain-compose/config.yaml" not in seen:
        raise ValueError("archive must contain state and configuration including config.yaml")
    return tf, valid


def main():
    if len(sys.argv) not in (3, 4) or sys.argv[1] not in ("validate", "extract"):
        raise SystemExit("usage: archive-safety.py validate ARCHIVE | extract ARCHIVE DEST")
    action, archive = sys.argv[1:3]
    tf, valid = members(archive)
    if action == "validate":
        print("Backup archive paths and file types are safe.")
        return
    if len(sys.argv) != 4:
        raise SystemExit("extract requires a destination")
    dest = pathlib.Path(sys.argv[3])
    dest.mkdir(mode=0o700, parents=True, exist_ok=False)
    try:
        for item, name in sorted(valid, key=lambda pair: (not pair[0].isdir(), pair[1].count("/"), pair[1])):
            target = dest.joinpath(*pathlib.PurePosixPath(name).parts)
            if item.isdir():
                target.mkdir(mode=0o700, parents=True, exist_ok=True)
                continue
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            source = tf.extractfile(item)
            if source is None:
                raise ValueError(f"cannot read archive member: {name!r}")
            flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
            if hasattr(os, "O_NOFOLLOW"):
                flags |= os.O_NOFOLLOW
            fd = os.open(target, flags, 0o600)
            with os.fdopen(fd, "wb") as output, source:
                shutil.copyfileobj(source, output)
            os.chmod(target, 0o600 | (item.mode & 0o100))
        for path in sorted(dest.rglob("*"), key=lambda p: len(p.parts), reverse=True):
            if path.is_dir():
                os.chmod(path, 0o700)
        os.chmod(dest, 0o700)
    except Exception:
        shutil.rmtree(dest, ignore_errors=True)
        raise
    finally:
        tf.close()
    print("Backup safely staged; ownership and access modes still require service-specific restore.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, tarfile.TarError, ValueError) as error:
        print(f"archive safety error: {error}", file=sys.stderr)
        raise SystemExit(1)
