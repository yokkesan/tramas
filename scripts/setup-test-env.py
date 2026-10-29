"""Create isolated, local-only test credentials without overwriting an existing file."""
from pathlib import Path
import os
import secrets

path = Path(__file__).resolve().parents[1] / '.env.test'
content = '\n'.join(f'{key}={secrets.token_hex(24)}' for key in (
    'TEST_OWNER_PASSWORD', 'TEST_APP_PASSWORD', 'TEST_MIGRATOR_PASSWORD',
)) + '\n'
try:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    raise SystemExit('.env.test already exists; left unchanged.')
with os.fdopen(fd, 'w') as file:
    file.write(content)
print('Created .env.test (0600), for isolated tests only.')
