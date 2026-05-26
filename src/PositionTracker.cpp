#include "PositionTracker.h"
#include <cmath>

double Position::unrealisedPnL(double currentPrice) const {
    return (currentPrice - avg_cost) * net_qty;
}

void PositionTracker::recordFill(const std::string& trader_id,
                                 Side side, double price, double qty) {
    auto& pos = positions_[trader_id];
    pos.trader_id = trader_id;
    pos.total_fills++;

    if (side == Side::BUY) {
        if (pos.net_qty >= 0.0) {
            // Adding to long or opening from flat
            pos.avg_cost = (pos.avg_cost * pos.net_qty + price * qty)
                           / (pos.net_qty + qty);
            pos.net_qty += qty;
        } else {
            // Currently short — buying to close (or flip)
            double short_qty = -pos.net_qty;
            if (qty >= short_qty) {
                // Close entire short: PnL = (entry - exit) * qty for shorts
                pos.realised_pnl += (pos.avg_cost - price) * short_qty;
                double remaining = qty - short_qty;
                pos.net_qty = 0.0;
                if (remaining > 0.0) {
                    pos.avg_cost = price;
                    pos.net_qty  = remaining;
                }
            } else {
                // Partially close short
                pos.realised_pnl += (pos.avg_cost - price) * qty;
                pos.net_qty += qty;
            }
        }
    } else {  // SELL
        if (pos.net_qty <= 0.0) {
            // Adding to short or opening short from flat
            double abs_qty = -pos.net_qty;
            pos.avg_cost = (pos.avg_cost * abs_qty + price * qty)
                           / (abs_qty + qty);
            pos.net_qty -= qty;
        } else {
            // Currently long — selling to close (or flip)
            double long_qty = pos.net_qty;
            if (qty >= long_qty) {
                // Close entire long
                pos.realised_pnl += (price - pos.avg_cost) * long_qty;
                double remaining = qty - long_qty;
                pos.net_qty = 0.0;
                if (remaining > 0.0) {
                    pos.avg_cost = price;
                    pos.net_qty  = -remaining;
                }
            } else {
                // Partially close long
                pos.realised_pnl += (price - pos.avg_cost) * qty;
                pos.net_qty -= qty;
            }
        }
    }
}

const Position* PositionTracker::get(const std::string& trader_id) const {
    auto it = positions_.find(trader_id);
    if (it == positions_.end()) return nullptr;
    return &it->second;
}

std::vector<Position> PositionTracker::snapshot() const {
    std::vector<Position> result;
    result.reserve(positions_.size());
    for (auto& [id, pos] : positions_)
        result.push_back(pos);
    return result;
}
