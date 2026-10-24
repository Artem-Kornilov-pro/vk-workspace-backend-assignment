#include <iostream>
#include <iterator>
#include <string>
#include <string_view>

#include "msgfmt/format.hpp"
#include "msgfmt/json.hpp"
#include "msgfmt/utf.hpp"

namespace {

constexpr int kExitOk = 0;
constexpr int kExitFailure = 1;
constexpr int kExitInvalidUtf8 = 2;

void write(std::string_view data) {
    std::cout.write(data.data(), static_cast<std::streamsize>(data.size()));
}

}  // namespace

int main(int argc, char* argv[]) {
    std::ios::sync_with_stdio(false);

    const bool escape_mode = argc == 2 && std::string_view(argv[1]) == "escape";
    if (argc > 2 || (argc == 2 && !escape_mode)) {
        std::cerr << "usage: msgfmt [escape] < input\n";
        return kExitFailure;
    }

    const std::string input{std::istreambuf_iterator<char>(std::cin),
                            std::istreambuf_iterator<char>()};
    const auto decoded = msgfmt::decode_utf8(input);
    if (!decoded) {
        write(msgfmt::error_json("invalid_utf8"));
        write("\n");
        std::cout.flush();
        return kExitInvalidUtf8;
    }

    if (escape_mode) {
        write(msgfmt::escape(*decoded));
    } else {
        write(msgfmt::to_json(msgfmt::parse(*decoded)));
        write("\n");
    }
    std::cout.flush();
    return std::cout ? kExitOk : kExitFailure;
}
