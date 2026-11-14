"""Copy the shared brand artwork and base styles into the Roundcube skin."""
from pathlib import Path
import shutil
root = Path(__file__).resolve().parents[1]
skin = root / 'webmail/skins/bestmaple'
(skin / 'assets').mkdir(parents=True, exist_ok=True)
for name in ['bestmaple-mark.svg', 'trade-routes.svg']:
    shutil.copyfile(root / 'core/frontend/src/assets/brand' / name, skin / 'assets' / name)
shutil.copyfile(root / 'core/frontend/src/views/login/login.css', skin / 'auth-base.css')
print('BESTMAPLE login assets synchronized')
