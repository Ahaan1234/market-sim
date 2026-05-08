#pragma once
#include <cstdint>

enum class Side { BUY, SELL };

struct Order {
    uint64_t id;
    Side     side;
    double   price;
    double   quantity;
    uint64_t timestamp;
};

struct Trade {
    uint64_t aggressor_id;
    uint64_t passive_id;
    double   price;
    double   quantity;
};
