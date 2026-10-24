#include <algorithm>
#include <array>
#include <utility>

#include "msgfmt/chars.hpp"
#include "msgfmt/format.hpp"
#include "msgfmt/utf.hpp"

namespace msgfmt {

std::string_view style_name(Style style) {
    switch (style) {
        case Style::Bold:
            return "bold";
        case Style::Italic:
            return "italic";
        case Style::Code:
            return "code";
        case Style::Link:
            return "link";
    }
    return "";
}

bool operator==(const Range& a, const Range& b) {
    return a.style == b.style && a.offset == b.offset && a.length == b.length && a.url == b.url;
}

namespace {

constexpr std::size_t kNone = std::u32string_view::npos;
constexpr std::size_t kStyleCount = 4;

std::size_t index_of(Style style) {
    return static_cast<std::size_t>(style);
}

// Результат последнего поиска закрывающего маркера одного стиля в сегменте.
// Поиск идёт из from (позиция сразу за открывающим маркером) и даёт found.
// Если из from ничего не найдено, то и из любой позиции правее ничего не
// найдётся; если найден found, то из любой позиции левее found найдётся он же.
// Повторное использование результата делает разбор линейным даже на входах
// вида «_a _a _a …», где у каждого открывающего маркера нет пары.
struct Memo {
    std::size_t from = kNone;
    std::size_t found = kNone;
    std::size_t url_end = kNone;  // только для ссылок: позиция `)` или kNone

    bool covers(std::size_t pos) const {
        return from != kNone && pos >= from && (found == kNone || pos < found);
    }
};

using Memos = std::array<Memo, kStyleCount>;

class Parser {
public:
    explicit Parser(std::u32string_view input) : in_(input) {}

    Document run() {
        parse_segment(0, in_.size());
        // Диапазоны добавлялись в порядке открытия, stable_sort его сохраняет
        // для полностью совпадающих диапазонов.
        std::stable_sort(ranges_.begin(), ranges_.end(), [](const Range& a, const Range& b) {
            if (a.offset != b.offset) {
                return a.offset < b.offset;
            }
            return a.length > b.length;
        });
        return Document{std::move(text_), std::move(ranges_)};
    }

private:
    // Разбирает in_[pos, end). Конструкция, начатая внутри сегмента, обязана
    // закрыться в нём же, поэтому диапазоны не пересекаются частично.
    void parse_segment(std::size_t pos, std::size_t end) {
        Memos memos;
        while (pos < end) {
            const char32_t c = in_[pos];
            std::size_t next = kNone;
            switch (c) {
                case U'\\':
                    if (pos + 1 < end && is_service_char(in_[pos + 1])) {
                        emit(in_[pos + 1]);
                        pos += 2;
                    } else {
                        emit(c);
                        ++pos;
                    }
                    continue;
                case U'*':
                    if (pos + 1 < end && in_[pos + 1] == U'*') {
                        next = try_bold(pos, end, memos[index_of(Style::Bold)]);
                        if (next == kNone) {
                            // Маркер `**` без пары выводится целиком.
                            emit(U'*');
                            emit(U'*');
                            pos += 2;
                            continue;
                        }
                    }
                    break;
                case U'_':
                    next = try_italic(pos, end, memos[index_of(Style::Italic)]);
                    break;
                case U'`':
                    next = try_code(pos, end, memos[index_of(Style::Code)]);
                    break;
                case U'[':
                    next = try_link(pos, end, memos[index_of(Style::Link)]);
                    break;
                default:
                    break;
            }
            if (next != kNone) {
                pos = next;
            } else {
                emit(c);
                ++pos;
            }
        }
    }

    std::size_t try_bold(std::size_t pos, std::size_t end, Memo& memo) {
        const std::size_t from = pos + 2;
        if (is_open(Style::Bold) || !starts_content(from, end)) {
            return kNone;
        }
        const std::size_t close = find(memo, from, [&] { return scan_bold(from, end); });
        if (close == kNone) {
            return kNone;
        }
        add_range(Style::Bold, from, close, {});
        return close + 2;
    }

    std::size_t try_italic(std::size_t pos, std::size_t end, Memo& memo) {
        const std::size_t from = pos + 1;
        const bool word_start = pos == 0 || !is_letter_or_digit(in_[pos - 1]);
        if (is_open(Style::Italic) || !word_start || !starts_content(from, end)) {
            return kNone;
        }
        const std::size_t close = find(memo, from, [&] { return scan_italic(from, end); });
        if (close == kNone) {
            return kNone;
        }
        add_range(Style::Italic, from, close, {});
        return close + 1;
    }

    std::size_t try_code(std::size_t pos, std::size_t end, Memo& memo) {
        const std::size_t from = pos + 1;
        if (!starts_content(from, end)) {
            return kNone;
        }
        const std::size_t close = find(memo, from, [&] { return scan_code(from, end); });
        if (close == kNone) {
            return kNone;
        }
        add_range(Style::Code, from, close, {});
        return close + 1;
    }

    // [текст](url): текст непустой и без `]`, url непустой, без пробельных
    // символов и без `)`; url берётся как есть.
    std::size_t try_link(std::size_t pos, std::size_t end, Memo& memo) {
        const std::size_t from = pos + 1;
        if (is_open(Style::Link) || !starts_content(from, end)) {
            return kNone;
        }
        if (!memo.covers(from)) {
            memo.from = from;
            memo.found = scan_bracket(from, end);
            memo.url_end = kNone;
            if (memo.found != kNone && memo.found > from && !is_whitespace(in_[memo.found - 1])) {
                memo.url_end = scan_url(memo.found, end);
            }
        }
        const std::size_t close = memo.found;
        const std::size_t url_end = memo.url_end;
        if (close == kNone || url_end == kNone) {
            return kNone;
        }
        add_range(Style::Link, from, close,
                  encode_utf8(in_.substr(close + 2, url_end - close - 2)));
        return url_end + 1;
    }

    template <typename Scan>
    static std::size_t find(Memo& memo, std::size_t from, Scan scan) {
        if (!memo.covers(from)) {
            memo.from = from;
            memo.found = scan();
        }
        return memo.found;
    }

    // Ближайший `**` в (from, end), перед которым непробельный символ.
    std::size_t scan_bold(std::size_t from, std::size_t end) const {
        for (std::size_t i = from; i + 1 < end; ++i) {
            if (is_escape_pair(i, end)) {
                ++i;
                continue;
            }
            if (i > from && in_[i] == U'*' && in_[i + 1] == U'*' && !is_whitespace(in_[i - 1])) {
                return i;
            }
        }
        return kNone;
    }

    // Ближайший `_` в (from, end), перед которым непробельный символ, а за
    // которым конец текста или не буква/цифра.
    std::size_t scan_italic(std::size_t from, std::size_t end) const {
        for (std::size_t i = from; i < end; ++i) {
            if (is_escape_pair(i, end)) {
                ++i;
                continue;
            }
            if (i > from && in_[i] == U'_' && !is_whitespace(in_[i - 1]) &&
                (i + 1 == in_.size() || !is_letter_or_digit(in_[i + 1]))) {
                return i;
            }
        }
        return kNone;
    }

    // Ближайший `` ` `` в (from, end), перед которым непробельный символ.
    // Внутри code экранирование не действует.
    std::size_t scan_code(std::size_t from, std::size_t end) const {
        for (std::size_t i = from + 1; i < end; ++i) {
            if (in_[i] == U'`' && !is_whitespace(in_[i - 1])) {
                return i;
            }
        }
        return kNone;
    }

    // Первый неэкранированный `]` в [from, end): текст ссылки не может содержать `]`,
    // поэтому другого кандидата на закрывающий нет.
    std::size_t scan_bracket(std::size_t from, std::size_t end) const {
        for (std::size_t i = from; i < end; ++i) {
            if (is_escape_pair(i, end)) {
                ++i;
                continue;
            }
            if (in_[i] == U']') {
                return i;
            }
        }
        return kNone;
    }

    // Позиция `)`, закрывающего url сразу после `](`, или kNone.
    std::size_t scan_url(std::size_t close, std::size_t end) const {
        if (close + 1 >= end || in_[close + 1] != U'(') {
            return kNone;
        }
        const std::size_t begin = close + 2;
        for (std::size_t i = begin; i < end; ++i) {
            if (in_[i] == U')') {
                return i > begin ? i : kNone;
            }
            if (is_whitespace(in_[i])) {
                return kNone;
            }
        }
        return kNone;
    }

    bool is_escape_pair(std::size_t i, std::size_t end) const {
        return in_[i] == U'\\' && i + 1 < end && is_service_char(in_[i + 1]);
    }

    bool starts_content(std::size_t from, std::size_t end) const {
        return from < end && !is_whitespace(in_[from]);
    }

    bool is_open(Style style) const {
        return open_[index_of(style)];
    }

    void add_range(Style style, std::size_t from, std::size_t close, std::string url) {
        const std::size_t index = ranges_.size();
        ranges_.push_back(Range{style, units_, 0, std::move(url)});
        open_[index_of(style)] = true;
        if (style == Style::Code) {
            for (std::size_t i = from; i < close; ++i) {
                emit(in_[i]);
            }
        } else {
            parse_segment(from, close);
        }
        open_[index_of(style)] = false;
        ranges_[index].length = units_ - ranges_[index].offset;
    }

    void emit(char32_t cp) {
        append_utf8(text_, cp);
        units_ += utf16_length(cp);
    }

    std::u32string_view in_;
    std::string text_;
    std::size_t units_ = 0;  // длина text_ в кодовых единицах UTF-16
    std::vector<Range> ranges_;
    std::array<bool, kStyleCount> open_{};
};

}  // namespace

Document parse(std::u32string_view input) {
    return Parser(input).run();
}

}  // namespace msgfmt
