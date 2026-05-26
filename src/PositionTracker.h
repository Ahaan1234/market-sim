#pragma once
#include <map>
#include <string>
#include <vector>
#include "Order.h"

struct Position {
    std::string trader_id;
    double net_qty      = 0.0;  // positive = long, negative = short
    double realised_pnl = 0.0;
    double avg_cost     = 0.0;  // volume-weighted average cost of current position
    int    total_fills  = 0;

    double unrealisedPnL(double currentPrice) const;
};

class PositionTracker {
public:
    void recordFill(const std::string& trader_id,
                    Side side, double price, double qty);

    const Position* get(const std::string& trader_id) const;

    std::vector<Position> snapshot() const;

private:
    std::map<std::string, Position> positions_;
};
