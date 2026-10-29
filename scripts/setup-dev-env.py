"""Create a local-only .env without replacing an existing file."""
from pathlib import Path
import os
import secrets

root = Path(__file__).resolve().parents[1]
content = (root / '.env.example').read_text()
content = content.replace('POSTGRES_PASSWORD=\n', f'POSTGRES_PASSWORD={secrets.token_hex(24)}\n')
content = content.replace('APP_DB_PASSWORD=\n', f'APP_DB_PASSWORD={secrets.token_hex(24)}\n')
content = content.replace('MIGRATION_DB_PASSWORD=\n', f'MIGRATION_DB_PASSWORD={secrets.token_hex(24)}\n')
try:
    fd = os.open(root / '.env', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    raise SystemExit('.env already exists; left unchanged.')
with os.fdopen(fd, 'w') as file:
    file.write(content)
print('Created .env (0600), for local development only.')
