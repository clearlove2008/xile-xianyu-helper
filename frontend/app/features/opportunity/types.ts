/** OpportunityItem 是一条商机搜索结果。 */
export interface OpportunityItem {
  /** itemId 是平台商品 ID。 */
  itemId: string;
  /** title 是商品标题。 */
  title: string;
  /** price 是价格文本。 */
  price: string;
  /** imageUrl 是封面图地址。 */
  imageUrl: string;
  /** itemUrl 是 PC 端商品页地址。 */
  itemUrl: string;
  /** seller 是卖家昵称。 */
  seller: string;
  /** area 是发货地。 */
  area: string;
}

/** OpportunitySearchResponse 是一次搜索的返回。 */
export interface OpportunitySearchResponse {
  success: boolean;
  items: OpportunityItem[];
  page: number;
  pageSize: number;
  total: number;
  hasMore: boolean;
}
