---
name: unicode-edge-cases
version: 1
author: международный-разработчик
confidence: 0.85
created: 2026-02-14
tags: [unicode, 中文, العربية, русский, 🔧]
dependencies: [базовая-настройка]
---

# Unicode Edge Cases 🌍

## When to Use
When dealing with internationalized content, emoji, and various Unicode edge cases:
- Non-Latin alphabets: 中文, العربية, русский, हिन्दी
- Emoji and symbols: 🚀 ⚡ 🔧 📚 ✨
- Special characters: café, naïve, résumé, piñata
- Right-to-left text: مرحبا بكم في المهارات الذكية

## Solution
Handle Unicode properly in all contexts:

```bash
# File names with Unicode
touch "файл-с-русским-именем.txt"
echo "Hello 世界!" > "unicode-файл.txt"

# Search with Unicode patterns
grep "世界" *.txt
find . -name "*файл*"
```

Python example:
```python
# -*- coding: utf-8 -*-
def process_unicode_text(text):
    """Process text with various Unicode characters."""
    # Normalize Unicode (NFC form)
    import unicodedata
    normalized = unicodedata.normalize('NFC', text)
    
    # Handle RTL text
    rtl_languages = ['ar', 'he', 'fa', 'ur']
    
    # Process emoji
    emoji_pattern = r'[\U0001F600-\U0001F64F\U0001F300-\U0001F5FF\U0001F680-\U0001F6FF\U0001F1E0-\U0001F1FF]'
    
    return {
        'original': text,
        'normalized': normalized,
        'length': len(text),
        'byte_length': len(text.encode('utf-8'))
    }

# Test with various inputs
examples = [
    "Hello 世界!",
    "Café naïve résumé",
    "مرحبا بالعالم",
    "Файл с русским именем",
    "🚀 Rocket science! 🔬"
]

for example in examples:
    result = process_unicode_text(example)
    print(f"Text: {result['original']}")
    print(f"Bytes: {result['byte_length']}")
```

## Gotchas
- **Normalization:** Different Unicode normalization forms (NFC, NFD, NFKC, NFKD)
- **Byte vs character length:** "café" has 4 characters but 5 bytes in UTF-8
- **Right-to-left text:** Can break terminal output and file operations
- **Emoji width:** Some emoji are wide characters (width=2) in terminals
- **Case folding:** Turkish i/I problem, German ß handling
- **Sorting:** Collation rules vary by language and locale
- **File systems:** Not all file systems support all Unicode characters
- **Zero-width characters:** Invisible characters that can break parsing: ​‌‍
- **Homoglyphs:** Different characters that look identical: а (Cyrillic) vs a (Latin)

## See Also
- [[file-encoding-detection]]
- [[locale-configuration]]
- [[internationalization-patterns]]
- [[text-normalization-strategies]]