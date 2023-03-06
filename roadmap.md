# autoscaler
1. scaler will be on node
2. scale to n+1 if any pods are on pending
3. scale down if any nodes usage is very less and can be deleted
4. while adding node check if high priority node can have more node. if yes create `there` not in current pool

# autoscaler (another approch)
1. if usage of any node  is very less check either it can be deleted
2. if in hole nodepool usage is more than theresold create node
...
