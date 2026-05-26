#pragma once
#include <optional>
#include <string>
#include "OrderQueue.h"

namespace protocol {
namespace detail {

// Extract a quoted-string value for `key` from a flat JSON line.
// Returns "" if the key is absent or the value is not a quoted string.
inline std::string extractString(const std::string& json, const std::string& key) {
    std::string needle = "\"" + key + "\":\"";
    auto pos = json.find(needle);
    if (pos == std::string::npos) return "";
    pos += needle.size();
    auto end = json.find('"', pos);
    if (end == std::string::npos) return "";
    return json.substr(pos, end - pos);
}

// Extract a numeric value for `key` from a flat JSON line.
// Returns false (leaves `out` unchanged) if the key is absent or not numeric.
inline bool extractNumber(const std::string& json, const std::string& key, double& out) {
    std::string needle = "\"" + key + "\":";
    auto pos = json.find(needle);
    if (pos == std::string::npos) return false;
    pos += needle.size();
    while (pos < json.size() && json[pos] == ' ') ++pos;
    if (pos >= json.size()) return false;
    try {
        std::size_t n;
        out = std::stod(json.substr(pos), &n);
        return (n > 0);
    } catch (...) {
        return false;
    }
}

} // namespace detail

// Parse one incoming order from a JSON line.
// Returns nullopt on any parse error; never throws.
// Expected format:
//   {"trader_id":"...","order_id":"...","side":"BUY|SELL","type":"LIMIT|MARKET","price":...,"qty":...}
// `price` is optional for MARKET orders (defaults to 0.0).
// `type`  is optional — defaults to LIMIT if absent.
inline std::optional<IncomingOrder> parseIncomingOrder(const std::string& line) noexcept {
    try {
        using namespace detail;

        std::string trader_id = extractString(line, "trader_id");
        std::string order_id  = extractString(line, "order_id");
        std::string side_str  = extractString(line, "side");
        std::string type_str  = extractString(line, "type");

        if (trader_id.empty() || order_id.empty() || side_str.empty()) return std::nullopt;

        double qty = 0.0;
        if (!extractNumber(line, "qty", qty)) return std::nullopt;

        Side side;
        if      (side_str == "BUY")  side = Side::BUY;
        else if (side_str == "SELL") side = Side::SELL;
        else return std::nullopt;

        OrderType type = (type_str == "MARKET") ? OrderType::MARKET : OrderType::LIMIT;

        double price = 0.0;
        extractNumber(line, "price", price);  // absent = 0.0; validated later for LIMIT

        return IncomingOrder{trader_id, order_id, side, type, price, qty};
    } catch (...) {
        return std::nullopt;
    }
}

} // namespace protocol
