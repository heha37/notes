# Byte Order Mark (BOM)

A _byte order mark_ (BOM) is a sequence of bytes used to indicate [Unicode](https://simple.wikipedia.org/wiki/Unicode) encoding of a text file. The underlying character code, `U+FEFF`, takes one of the following forms depending on the character encoding.

| Bytes | Encoding Form |
| :---: | :---: |
| EF BB BF | UTF-8 |
| FE FF | UTF-16, big-endian |
| FF FE | UTF-16, little-endian |
| 00 00 FE FF | UTF-32, big-endian |
| FF FE 00 00 | UTF-32, little-endian |


BOM use is optional. If used, it must be at the very beginning of the text. The BOM gives the producer of the text a way to describe the encoding such as UTF-8 or UTF-16, and in the case of UTF-16 and UTF-32, its endianness. The BOM is important for text interchange, when files move between systems that use different byte orders or different encodings, rather than in normal text handling in a closed environment.

As UTF-8 has become the most common text encoding, `EFBBBF` (shown here as three hexadecimal values) is the most commonly occurring BOM form, also known as the _UTF-8 signature_. HTML5 browsers are required to recognize the UTF-8 BOM and use it to detect the encoding of the page. Software may alternatively recognize UTF-8 encoding by looking for bytes with the high order bit set (values `0x80` through `0xFF`) followed by bytes that define valid UTF-8 sequences.

The Unicode Standard neither requires nor recommends the use of the BOM for UTF-8, but warns that it may be encountered at the start of a file.

**_一般存成无BOM的UTF-8文件_**
