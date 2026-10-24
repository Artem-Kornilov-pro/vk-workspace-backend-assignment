#pragma once

#include <cstddef>
#include <string>
#include <string_view>
#include <vector>

namespace msgfmt {

enum class Style { Bold, Italic, Code, Link };

std::string_view style_name(Style style);

// Диапазон стиля в итоговом тексте. offset и length — в кодовых единицах UTF-16,
// url непуст только у ссылок.
struct Range {
    Style style;
    std::size_t offset;
    std::size_t length;
    std::string url;
};

bool operator==(const Range& a, const Range& b);

struct Document {
    std::string text;  // UTF-8
    std::vector<Range> ranges;
};

// Разбирает разметку и возвращает текст без служебной разметки и диапазоны стилей.
Document parse(std::u32string_view input);

// Предваряет каждый служебный символ обратной косой чертой. Результат — UTF-8.
std::string escape(std::u32string_view input);

}  // namespace msgfmt
