#include "msgfmt/chars.hpp"

#include <algorithm>
#include <iterator>

namespace msgfmt {

namespace {

struct CodeRange {
    char32_t first;
    char32_t last;
};

// Буквы и цифры за пределами ASCII. Таблица упрощённая: покрывает основные
// алфавиты и не претендует на полноту категорий L* и Nd из Unicode.
// Диапазоны отсортированы и не пересекаются.
constexpr CodeRange kLettersAndDigits[] = {
    {0x00AA, 0x00AA},    // ª
    {0x00B5, 0x00B5},    // µ
    {0x00BA, 0x00BA},    // º
    {0x00C0, 0x00D6},    // Latin-1: буквы
    {0x00D8, 0x00F6},    //
    {0x00F8, 0x02AF},    // Latin-1, Latin Extended-A/B, IPA
    {0x0300, 0x036F},    // комбинируемые диакритические знаки — часть слова
    {0x0370, 0x0373},    // греческий
    {0x0376, 0x0377},    //
    {0x037B, 0x037D},    //
    {0x037F, 0x037F},    //
    {0x0386, 0x0386},    //
    {0x0388, 0x03FF},    //
    {0x0400, 0x0481},    // кириллица
    {0x048A, 0x052F},    //
    {0x0531, 0x0556},    // армянский
    {0x0560, 0x0588},    //
    {0x05D0, 0x05EA},    // иврит
    {0x05EF, 0x05F2},    //
    {0x0620, 0x064A},    // арабский
    {0x0660, 0x0669},    //
    {0x066E, 0x06D3},    //
    {0x06F0, 0x06FC},    //
    {0x0904, 0x0939},    // деванагари
    {0x0966, 0x096F},    //
    {0x0E01, 0x0E30},    // тайский
    {0x0E50, 0x0E59},    //
    {0x10A0, 0x10FA},    // грузинский
    {0x10FC, 0x10FF},    //
    {0x1E00, 0x1FFF},    // Latin Extended Additional, Greek Extended
    {0x3041, 0x3096},    // хирагана
    {0x30A1, 0x30FA},    // катакана
    {0x3400, 0x4DBF},    // CJK Extension A
    {0x4E00, 0x9FFF},    // CJK Unified Ideographs
    {0xAC00, 0xD7A3},    // хангыль
    {0xFF10, 0xFF19},    // полноширинные цифры
    {0xFF21, 0xFF3A},    // полноширинная латиница
    {0xFF41, 0xFF5A},    //
    {0x20000, 0x2A6DF},  // CJK Extension B
};

}  // namespace

bool is_whitespace(char32_t cp) {
    switch (cp) {
        case 0x0009:
        case 0x000A:
        case 0x000B:
        case 0x000C:
        case 0x000D:
        case 0x0020:
        case 0x0085:
        case 0x00A0:
        case 0x1680:
        case 0x2028:
        case 0x2029:
        case 0x202F:
        case 0x205F:
        case 0x3000:
            return true;
        default:
            return cp >= 0x2000 && cp <= 0x200A;
    }
}

bool is_letter_or_digit(char32_t cp) {
    if (cp < 0x80) {
        return (cp >= U'0' && cp <= U'9') || (cp >= U'A' && cp <= U'Z') ||
               (cp >= U'a' && cp <= U'z');
    }
    const auto* it = std::upper_bound(
        std::begin(kLettersAndDigits), std::end(kLettersAndDigits), cp,
        [](char32_t value, const CodeRange& range) { return value < range.first; });
    if (it == std::begin(kLettersAndDigits)) {
        return false;
    }
    --it;
    return cp <= it->last;
}

bool is_service_char(char32_t cp) {
    switch (cp) {
        case U'*':
        case U'_':
        case U'`':
        case U'[':
        case U']':
        case U'(':
        case U')':
        case U'\\':
            return true;
        default:
            return false;
    }
}

}  // namespace msgfmt
