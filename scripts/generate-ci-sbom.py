#!/usr/bin/env python3
"""Generate an SPDX 2.3 inventory of direct release-build CI inputs."""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import platform
import re
import subprocess
from pathlib import Path


def command_output(*args: str) -> str:
    try:
        result = subprocess.run(args, check=True, text=True, capture_output=True)
        return (result.stdout or result.stderr).strip()
    except (OSError, subprocess.CalledProcessError):
        return "unavailable"


def command(*args: str) -> str:
    return command_output(*args).splitlines()[0]


def os_release() -> str:
    path = Path('/etc/os-release')
    if path.exists():
        values = {}
        for line in path.read_text(encoding='utf-8').splitlines():
            if '=' in line:
                key, value = line.split('=', 1)
                values[key] = value.strip().strip('"')
        return f"{values.get('PRETTY_NAME', 'unknown')} ({values.get('VERSION_ID', 'unknown')})"
    return platform.platform()


def release_build_actions(workflow: Path) -> list[tuple[str, str, str]]:
    text = workflow.read_text(encoding='utf-8')
    marker = '  release-build-repro:\n'
    if marker not in text:
        raise ValueError('release-build-repro job not found in workflow')
    section = text.split(marker, 1)[1].split('\n  independent-rebuild-check:', 1)[0]
    actions: list[tuple[str, str, str]] = []
    for line in section.splitlines():
        use = re.search(r'\buses:\s*([^\s#]+)', line)
        if not use:
            continue
        ref = use.group(1)
        if ref.startswith('./') or '@' not in ref:
            continue
        repo, version = ref.rsplit('@', 1)
        url = repo if repo.startswith('docker://') else f'https://github.com/{repo}'
        actions.append((repo, version, url))
    return sorted(set(actions))


def package(name: str, version: str, download: str, purpose: str = 'APPLICATION') -> dict:
    safe = re.sub(r'[^A-Za-z0-9.-]+', '-', name).strip('-') or 'package'
    suffix = hashlib.sha256(f'{name}@{version}'.encode()).hexdigest()[:10]
    return {
        'name': name,
        'SPDXID': f'SPDXRef-{safe[:48]}-{suffix}',
        'versionInfo': version,
        'downloadLocation': download or 'NOASSERTION',
        'filesAnalyzed': False,
        'licenseConcluded': 'NOASSERTION',
        'licenseDeclared': 'NOASSERTION',
        'copyrightText': 'NOASSERTION',
        'primaryPackagePurpose': purpose,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--workflow', default='.github/workflows/ci.yml')
    parser.add_argument('--output', required=True)
    parser.add_argument('--manifest', required=True)
    parser.add_argument('--syft-version', default='')
    args = parser.parse_args()

    workflow = Path(args.workflow)
    actions = release_build_actions(workflow)
    packages = []
    for name, version, url in actions:
        packages.append(package(name, version, url))

    go_version = command('go', 'version')
    git_version = command('git', '--version')
    bash_version = command('bash', '--version')
    python_version = command('python3', '--version')
    tar_version = command('tar', '--version')
    checksum_version = command('sha256sum', '--version')
    syft_version = args.syft_version or 'not used in this job'
    runner_label = 'ubuntu-24.04'
    runner_image = f"{os.environ.get('ImageOS', 'unknown')} {os.environ.get('ImageVersion', 'unknown')}"

    tools = [
        ('Go toolchain', go_version, 'https://go.dev/dl/'),
        ('Git', git_version, 'https://git-scm.com/'),
        ('Bash', bash_version, 'https://www.gnu.org/software/bash/'),
        ('Python', python_version, 'https://www.python.org/'),
        ('tar', tar_version, 'https://www.gnu.org/software/tar/'),
        ('sha256sum (coreutils)', checksum_version, 'https://www.gnu.org/software/coreutils/'),
        ('Syft', syft_version, 'https://github.com/anchore/syft'),
        ('GitHub-hosted runner image', runner_image, 'https://github.com/actions/runner-images'),
    ]
    packages.extend(package(name, version, url, 'OPERATING-SYSTEM' if name == 'GitHub-hosted runner image' else 'APPLICATION') for name, version, url in tools)

    created = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace('+00:00', 'Z')
    digest_input = json.dumps(packages, sort_keys=True).encode()
    namespace_id = hashlib.sha256(digest_input + created.encode()).hexdigest()
    spdx_ids = [item['SPDXID'] for item in packages]
    document = {
        'spdxVersion': 'SPDX-2.3',
        'dataLicense': 'CC0-1.0',
        'SPDXID': 'SPDXRef-DOCUMENT',
        'name': 'NEXUS-Q key provider release-build CI environment',
        'documentNamespace': f'https://spdx.org/spdxdocs/nexusq-ci-environment-{namespace_id}',
        'creationInfo': {
            'creators': ['Tool: nexusq-keyprovider-ci-sbom-generator'],
            'created': created,
            'comment': 'Direct release-build workflow actions and directly invoked build tools; not a full inventory of preinstalled runner packages or transitive action dependencies.',
        },
        'packages': packages,
        'documentDescribes': spdx_ids,
        'relationships': [
            {'spdxElementId': 'SPDXRef-DOCUMENT', 'relationshipType': 'DESCRIBES', 'relatedSpdxElement': item_id}
            for item_id in spdx_ids
        ],
    }
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(document, indent=2) + '\n', encoding='utf-8')

    manifest = [
        'NEXUS-Q key provider release-build environment',
        f'Runner label: {runner_label}',
        f'Runner image: {runner_image}',
        f'OS: {os_release()}',
        f'Kernel/architecture: {platform.platform()} / {platform.machine()}',
        f'Go: {go_version}',
        f'Go environment: {command_output("go", "env", "GOOS", "GOARCH", "CGO_ENABLED", "GOTOOLCHAIN", "GOWORK", "GOFLAGS").replace(chr(10), ", ")}',
        f'Go module graph: {command_output("go", "list", "-m", "all").replace(chr(10), ", ")}',
        f'Git: {git_version}',
        f'Bash: {bash_version}',
        f'Python: {python_version}',
        f'tar: {tar_version}',
        f'sha256sum: {checksum_version}',
        f'Syft: {syft_version}',
        'Direct actions used by release-build-repro:',
    ]
    manifest.extend(f'  - {name}@{version}' for name, version, _ in actions)
    manifest.extend([
        '',
        'Scope: directly declared release-build actions and tools invoked by the release build.',
        'Excludes: all preinstalled runner packages, transitive dependencies inside actions, GitHub service internals, and external NEXUS-Q deployment software.',
    ])
    Path(args.manifest).parent.mkdir(parents=True, exist_ok=True)
    Path(args.manifest).write_text('\n'.join(manifest) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
