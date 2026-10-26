#!/usr/bin/env python3
"""Explicit reviewed wiring step for the three-node reference qualification."""
import json
import os
import pathlib
import sys

from backup_helpers import owned_file, port_free


def main():
    root = pathlib.Path(sys.argv[1]).resolve()
    private = root / 'private'
    verify_only = sys.argv[2:] == ['--verify-only']
    for name in ('backend-tokens.json', 'air-reference-token', 'router-config.json', 'live-manifest.json'):
        owned_file(private / name, 0o600)
    tokens = json.loads((private / 'backend-tokens.json').read_text())
    tokens['air'] = (private / 'air-reference-token').read_text().strip()
    if len(set(tokens.values())) != 3:
        raise ValueError('three independent tokens required')
    config = json.loads((private / 'router-config.json').read_text())
    expected = [{'name': 'air', 'port': 19487, 'token_key': 'air'},
                {'name': 'this-mac', 'port': 19488, 'token_key': 'this-mac'},
                {'name': 'other-pro', 'port': 19489, 'token_key': 'other-pro'}]
    path = private / 'pool-backend-tokens.json'
    if verify_only:
        owned_file(path, 0o600)
        if json.loads(path.read_text()) != tokens or config['backends'] != expected or config['backend_tokens'] != str(path):
            raise ValueError('three-node wiring differs from reviewed scope')
    else:
        port_free(19495)
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as stream:
            json.dump(tokens, stream)
        config.update(backends=expected, backend_tokens=str(path))
        (private / 'router-config.json').write_text(json.dumps(config))
    manifest = json.loads((private / 'live-manifest.json').read_text())
    manifest['actor_roles'] = ['router', 'fault-proxy', 'air-reference']
    (private / 'live-manifest.json').write_text(json.dumps(manifest))
    print(json.dumps({'three_node_wiring_verified': True, 'backends': [x['name'] for x in expected],
                      'air_path': 'read-only reference to shared production helper via AWS19085; no production route changes'}))


if __name__ == '__main__':
    main()
