"""Validate local documentation links, capture provenance and generated PDFs."""
from pathlib import Path
import hashlib
import json
import re
import sys
import fitz

ROOT = Path(__file__).resolve().parents[2]
HANDBOOK = ROOT / 'docs' / 'handbook'
failures = []

markdown = list(HANDBOOK.rglob('*.md')) + [ROOT / 'README.md', ROOT / 'core' / 'README.MD', ROOT / 'core' / 'frontend' / 'README.md']
for document in markdown:
    if not document.exists():
        failures.append(f'Missing Markdown: {document.relative_to(ROOT)}')
        continue
    source = document.read_text(encoding='utf-8')
    for destination in re.findall(r'!?\[[^\]]*\]\(([^)]+)\)', source):
        if destination.startswith(('https://', 'http://', 'mailto:', '#')):
            continue
        destination = destination.split('#', 1)[0]
        target = (document.parent / destination).resolve()
        # This report is generated at the end of the current validation run.
        if target == (HANDBOOK / 'validation-report.json').resolve():
            continue
        if not target.exists():
            failures.append(f'Broken link in {document.relative_to(ROOT)}: {destination}')

provenance = json.loads((HANDBOOK / 'assets' / 'screenshot-provenance.json').read_text(encoding='utf-8'))
if provenance['production_access'] or provenance['external_requests_allowed'] or provenance['errors']:
    failures.append('Screenshot provenance contains production access or browser errors')
if len(provenance['captures']) != 32:
    failures.append('Expected 32 bilingual interface captures')
for capture in provenance['captures']:
    image = HANDBOOK / 'assets' / 'screenshots' / capture['filename']
    if not image.exists() or image.stat().st_size < 10000:
        failures.append(f'Missing/empty screenshot: {image.name}')

results = []
for locale in ['zh-CN', 'en']:
    document = HANDBOOK / 'pdf' / f'LeadPilotAI-User-and-Operations-Guide-{locale}.pdf'
    pdf = fitz.open(document)
    text = '\n'.join(page.get_text() for page in pdf)
    images = sum(len(page.get_images()) for page in pdf)
    landscape = sum(page.rect.width > page.rect.height for page in pdf)
    if len(pdf) < 25 or images < 16 or landscape < 19:
        failures.append(f'{document.name}: insufficient pages/images/landscape figures')
    required = ['技术架构', '功能架构', '项目操作手册', '故障处理'] if locale == 'zh-CN' else ['Technical Architecture', 'Functional Architecture', 'Project User Manual', 'Troubleshooting']
    for phrase in required:
        if phrase not in text:
            failures.append(f'{document.name}: missing searchable text {phrase}')
    blank_pages = [i + 1 for i, page in enumerate(pdf) if len(page.get_text().strip()) < 70 and not page.get_images()]
    if blank_pages:
        failures.append(f'{document.name}: possible blank pages {blank_pages}')
    results.append({'locale': locale, 'pdf': str(document.relative_to(ROOT)).replace('\\', '/'), 'pages': len(pdf), 'landscape_pages': landscape, 'image_instances': images, 'bytes': document.stat().st_size, 'sha256': hashlib.sha256(document.read_bytes()).hexdigest()})
    pdf.close()

report = {'source_baseline': 'cf57fb3', 'capture_count': len(provenance['captures']), 'markdown_files_checked': len(markdown), 'pdfs': results, 'errors': failures}
(HANDBOOK / 'validation-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps(report, ensure_ascii=False, indent=2))
sys.exit(1 if failures else 0)
